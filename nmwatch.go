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
	PROC_CN_MCAST_LISTEN = 1
	PROC_CN_MCAST_IGNORE = 2

	PROC_EVENT_FORK = 0x00000001 // fork() events
	PROC_EVENT_EXEC = 0x00000002 // exec() events
	PROC_EVENT_NM   = 0x00000400 // namespace events
	PROC_EVENT_EXIT = 0x80000000 // exit() events
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
type nmProcEvent struct {
	ProcessPid  uint32
	ProcessTgid uint32
	NmType      uint32
	NmReason    uint32
	OldInum     uint64
	Inum        uint64
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

func subscribe(sock int, addr *syscall.SockaddrNetlink) {
	var op uint32
	op = PROC_CN_MCAST_LISTEN
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
				handleEvent(m.Data)
			}
		}
	}
}

func handleEvent(data []byte) {
	buf := bytes.NewBuffer(data)
	msg := &cnMsg{}
	hdr := &procEventHeader{}

	binary.Read(buf, byteOrder, msg)
	binary.Read(buf, byteOrder, hdr)

	switch hdr.What {
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

	case PROC_EVENT_NM:
		event := &nmProcEvent{}
		binary.Read(buf, byteOrder, event)
		pid := int(event.ProcessTgid)
		nmType := int(event.NmType)
		nmReason := int(event.NmReason)
		oldInum := uint64(event.OldInum)
		inum := uint64(event.Inum)

		var typeStr string
		switch nmType {
		case syscall.CLONE_NEWPID:
			typeStr = "pid"
		case syscall.CLONE_NEWNS:
			typeStr = "mnt"
		case syscall.CLONE_NEWNET:
			typeStr = "net"
		case syscall.CLONE_NEWUTS:
			typeStr = "uts"
		case syscall.CLONE_NEWIPC:
			typeStr = "ipc"
		case syscall.CLONE_NEWUSER:
			typeStr = "user"
		default:
			typeStr = "unknown"
		}
		fmt.Printf("nm: pid=%v type=%v reason=%v old_inum=%v inum=%v\n", pid, typeStr, nmReason, oldInum, inum)

	case PROC_EVENT_EXIT:
		event := &exitProcEvent{}
		binary.Read(buf, byteOrder, event)
		pid := int(event.ProcessTgid)

		fmt.Printf("exit: pid=%v\n", pid)
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

	subscribe(sock, addr)

	receive(sock)
}
