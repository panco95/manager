## Manager

A server manager used by ETCD.

## Example

```
go get -u github.com/panco95/manager

manager, err := manager.NewManager(
	pkgs.etcdClient.Client,
	"",
	"9000",
	"test",
)
servers, err := manager.GetAllServices()
fmt.Println(servers)
```
