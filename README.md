# nmwatch

Experimental code to use the Netlink proc connector interface to get information about new processes and new namespaces.

Requires:
- `CONFIG_PROC_EVENTS=y`: Netlink proc connector
- [kernel patch to add namespace events in the proc connector](https://github.com/kinvolk/linux/commits/alban/cn_proc_nm)

## Example

```
$ sudo go run nmwatch.go
Hello
```

While it is running, in a different terminal:
```
# unshare -n -f ls -l /proc/self/ns/net
lrwxrwxrwx 1 root root 0 Sep  6 05:35 /proc/self/ns/net -> 'net:[4026532142]'
```

Observe the fork and exec events caused by the `unshare` command and the creation of a new network namespace:
```
fork: ppid=696 pid=858
exec: pid=858
nm: pid=858 type=net reason=0 old_inum=4026531957 inum=4026532142
fork: ppid=858 pid=859
exec: pid=859
exit: pid=859
exit: pid=858
```

