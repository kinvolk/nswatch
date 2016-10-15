/* With code inspired from
 * https://github.com/cloudfoundry/gosigar/tree/master/psnotify
 * with additions for namespace support
 */

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
)

const (
	// <linux/connector.h>
	CN_IDX_PROC = 0x1
	CN_VAL_PROC = 0x1

	// <linux/cn_proc.h>
	PROC_CN_GET_FEATURES = 0
	PROC_CN_MCAST_LISTEN = 1
	PROC_CN_MCAST_IGNORE = 2

	PROC_EVENT_NONE   = 0x00000000
	PROC_EVENT_FORK   = 0x00000001
	PROC_EVENT_EXEC   = 0x00000002
	PROC_EVENT_UID    = 0x00000004
	PROC_EVENT_GID    = 0x00000040
	PROC_EVENT_SID    = 0x00000080
	PROC_EVENT_PTRACE = 0x00000100
	PROC_EVENT_COMM   = 0x00000200
	PROC_EVENT_NS     = 0x00000400
	/* "next" should be 0x00000800 */
	/* "last" is the last process event: exit,
	 * while "next to last" is coredumping event */
	PROC_EVENT_COREDUMP = 0x40000000
	PROC_EVENT_EXIT     = 0x80000000
)

var (
	byteOrder = binary.LittleEndian

	seq uint32
)

// linux/connector.h: struct cb_id
type cbId struct {
	Idx uint32
	Val uint32
}

// linux/connector.h: struct cb_msg
type cnMsg struct {
	Id    cbId
	Seq   uint32
	Ack   uint32
	Len   uint16
	Flags uint16
}

// linux/cn_proc.h: struct proc_event.{what,cpu,timestamp_ns}
type procEventHeader struct {
	What      uint32
	Cpu       uint32
	Timestamp uint64
}

type namespaceEventHeader struct {
	Timestamp   uint64
	ProcessPid  uint32
	ProcessTgid uint32
	Reason      uint32
	Count       uint32
}

type namespaceEventContent struct {
	Type    uint32
	Flags   uint32
	OldInum uint64
	Inum    uint64
}

// linux/cn_proc.h: struct proc_event.fork
type forkProcEvent struct {
	ParentPid  uint32
	ParentTgid uint32
	ChildPid   uint32
	ChildTgid  uint32
}

// linux/cn_proc.h: struct proc_event.exec
type execProcEvent struct {
	ProcessPid  uint32
	ProcessTgid uint32
}

// linux/cn_proc.h: struct proc_event.exec
type nsProcItem struct {
	ItemType uint32
	Flag     uint32
	OldInum  uint64
	Inum     uint64
}
type nsProcEvent struct {
	ProcessPid  uint32
	ProcessTgid uint32
	Reason      uint32
	Count       uint32
	Items       [7]nsProcItem
}

// linux/cn_proc.h: struct proc_event.exit
type exitProcEvent struct {
	ProcessPid  uint32
	ProcessTgid uint32
	ExitCode    uint32
	ExitSignal  uint32
}

// standard netlink header + connector header
type netlinkProcMessage struct {
	Header syscall.NlMsghdr
	Data   cnMsg
}

func subscribe(sock int, addr *syscall.SockaddrNetlink, op uint32) {
	seq++

	pr := &netlinkProcMessage{}
	plen := binary.Size(pr.Data) + binary.Size(op)
	pr.Header.Len = syscall.NLMSG_HDRLEN + uint32(plen)
	pr.Header.Type = uint16(syscall.NLMSG_DONE)
	pr.Header.Flags = 0
	pr.Header.Seq = seq
	pr.Header.Pid = uint32(os.Getpid())

	pr.Data.Id.Idx = CN_IDX_PROC
	pr.Data.Id.Val = CN_VAL_PROC

	pr.Data.Len = uint16(binary.Size(op))

	buf := bytes.NewBuffer(make([]byte, 0, pr.Header.Len))
	binary.Write(buf, byteOrder, pr)
	binary.Write(buf, byteOrder, op)

	err := syscall.Sendto(sock, buf.Bytes(), 0, addr)
	if err != nil {
		fmt.Printf("sendto failed: %v\n", err)
		os.Exit(1)
	}
}

func receive(sock int) {
	buf := make([]byte, syscall.Getpagesize())

	for {
		nr, _, err := syscall.Recvfrom(sock, buf, 0)
		if err != nil {
			fmt.Printf("recvfrom failed: %v\n", err)
			os.Exit(1)
		}
		if nr < syscall.NLMSG_HDRLEN {
			continue
		}

		msgs, _ := syscall.ParseNetlinkMessage(buf[:nr])
		for _, m := range msgs {
			if m.Header.Type == syscall.NLMSG_DONE {
				handleProcEvent(m.Data)
			}
		}
	}
}

func handleProcEvent(data []byte) {
	buf := bytes.NewBuffer(data)
	msg := &cnMsg{}
	hdr := &procEventHeader{}

	binary.Read(buf, byteOrder, msg)
	binary.Read(buf, byteOrder, hdr)

	switch hdr.What {
	case PROC_EVENT_NONE:
		fmt.Printf("none: flags=%v\n", msg.Flags)

	case PROC_EVENT_FORK:
		event := &forkProcEvent{}
		binary.Read(buf, byteOrder, event)
		ppid := int(event.ParentTgid)
		pid := int(event.ChildTgid)

		fmt.Printf("fork: ppid=%v pid=%v\n", ppid, pid)

	case PROC_EVENT_EXEC:
		event := &execProcEvent{}
		binary.Read(buf, byteOrder, event)
		pid := int(event.ProcessTgid)

		fmt.Printf("exec: pid=%v\n", pid)

	case PROC_EVENT_NS:
		event := &nsProcEvent{}
		binary.Read(buf, byteOrder, event)
		pid := int(event.ProcessTgid)
		count := int(event.Count)
		reason := int(event.Reason)

		var reasonStr string
		switch reason {
		case 1:
			reasonStr = "clone"
		case 2:
			reasonStr = "setns"
		case 3:
			reasonStr = "unshare"
		default:
			reasonStr = "unknown"
		}

		fmt.Printf("ns: pid=%v reason=%v count=%v\n", pid, reasonStr, count)

		for i := 0; i < count; i++ {

			itemType := uint64(event.Items[i].ItemType)
			oldInum := uint64(event.Items[i].OldInum)
			inum := uint64(event.Items[i].Inum)

			var typeStr string
			switch itemType {
			case syscall.CLONE_NEWPID:
				typeStr = "pid "
			case syscall.CLONE_NEWNS:
				typeStr = "mnt "
			case syscall.CLONE_NEWNET:
				typeStr = "net "
			case syscall.CLONE_NEWUTS:
				typeStr = "uts "
			case syscall.CLONE_NEWIPC:
				typeStr = "ipc "
			case syscall.CLONE_NEWUSER:
				typeStr = "user"
			default:
				typeStr = "unknown"
			}

			fmt.Printf("    type=%s %v -> %v\n", typeStr, oldInum, inum)
		}

	case PROC_EVENT_EXIT:
		event := &exitProcEvent{}
		binary.Read(buf, byteOrder, event)
		pid := int(event.ProcessTgid)

		fmt.Printf("exit: pid=%v\n", pid)

	case PROC_EVENT_UID:
	case PROC_EVENT_GID:
	case PROC_EVENT_SID:
	case PROC_EVENT_PTRACE:
	case PROC_EVENT_COMM:
	case PROC_EVENT_COREDUMP:

	default:
		fmt.Printf("???: what=%x\n", hdr.What)
	}
}

func main() {
	fmt.Printf("Hello\n")

	sock, err := syscall.Socket(
		syscall.AF_NETLINK,
		syscall.SOCK_DGRAM,
		syscall.NETLINK_CONNECTOR)
	if err != nil {
		fmt.Printf("socket failed: %v\n", err)
		os.Exit(1)
	}

	addr := &syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Groups: CN_IDX_PROC,
	}

	err = syscall.Bind(sock, addr)
	if err != nil {
		fmt.Printf("bind failed: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "sub":
			subscribe(sock, addr, PROC_CN_MCAST_LISTEN)
		case "unsub":
			subscribe(sock, addr, PROC_CN_MCAST_IGNORE)
		case "features":
			subscribe(sock, addr, PROC_CN_GET_FEATURES)
		}
	}

	receive(sock)
}
