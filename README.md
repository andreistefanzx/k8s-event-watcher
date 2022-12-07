# k8s-event-watcher

```
# download packages
go mod init
go mod tidy

# watch events across all namespaces in dry-run mode
go run main.go --dry-run

# watch events across a specific namespace in dry-run mode
go run main.go --dry-run --namespace "default"
```