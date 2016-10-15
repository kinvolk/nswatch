# nswatch

Experimental code to use the Netlink proc connector interface to get information about new processes and new namespaces.

Requires:
- `CONFIG_PROC_EVENTS=y`: Netlink proc connector, added by [commit 9f4608](https://github.com/torvalds/linux/commit/9f46080c41d5f3f7c00b4e169ba4b0b2865258bf) in v2.6.15 released on 3 January, 2006
- [kernel patches to add namespace events in the proc connector](https://github.com/kinvolk/linux/commits/alban/proc_ns_connector-v2-5)

## Example

```
$ sudo go run nswatch.go
```

While it is running, in a different terminal:
```
# unshare -n -i -f sleep 500
```

Observe the fork and exec events caused by the `unshare` command and the creation of a new network namespace:
```
fork: ppid=670 pid=1151
exec: pid=1151
ns: pid=1151 reason=unshare count=2
    type=ipc  4026531839 -> 4026532141
    type=net  4026531957 -> 4026532143
fork: ppid=1151 pid=1152
exec: pid=1152
```

Check with setns events:
```
nsenter -t 1152 -i -n sleep 500
```

Events generated:
```
fork: ppid=670 pid=1166
exec: pid=1166
ns: pid=1166 reason=setns count=1
    type=ipc  4026531839 -> 4026532141
ns: pid=1166 reason=setns count=1
    type=net  4026531957 -> 4026532143
exec: pid=1166
```

Check with clone events:
```
systemd-nspawn --image=Fedora-Cloud-Base-24-1.2.x86_64.raw --private-users --private-net
```

Events generated:
```
fork: ppid=670 pid=1171
exec: pid=1171
fork: ppid=2 pid=1172
fork: ppid=193 pid=1173
fork: ppid=1171 pid=1174
ns: pid=1174 reason=clone count=1
    type=mnt  4026531840 -> 4026532141
exit: pid=1173
fork: ppid=2 pid=1175
fork: ppid=2 pid=1176
fork: ppid=1174 pid=1177
ns: pid=1177 reason=clone count=6
    type=user 4026531837 -> 4026532148
    type=uts  4026531838 -> 4026532150
    type=ipc  4026531839 -> 4026532151
    type=mnt  4026532141 -> 4026532149
    type=pid  4026531836 -> 4026532152
    type=net  4026531957 -> 4026532154
exit: pid=1174
```

