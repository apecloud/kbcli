---
title: kbcli cluster create clickhouse
---

Create a clickhouse cluster.

```
kbcli cluster create clickhouse NAME [flags]
```

### Examples

```
  # Create a cluster with the default values
  kbcli cluster create clickhouse
  
  # Create a cluster with the specified cpu, memory and storage
  kbcli cluster create clickhouse --cpu 1 --memory 2 --storage 10
```

### Options

```
      --cpu float                          The amount of CPU requested for each ClickHouse instance, can be a number or string with units. Value range [0.5, 64]. (default 1)
      --disable-exporter                   Enable or disable monitor. (default true)
      --dry-run string[="unchanged"]       Must be "client", or "server". If with client strategy, only print the object that would be sent, and no data is actually sent. If with server strategy, submit the server-side request, but no data is persistent. (default "none")
      --edit                               Edit the API resource before creating
  -h, --help                               help for clickhouse
      --keeper.cpu string                  The amount of CPU requested for each ClickHouse Keeper instance, can be a number or string with units. Value range [0.1, 64].
      --keeper.memory string               The amount of memory requested for each ClickHouse Keeper instance, unit is Gi. Value range [0.5, 1000].
      --keeper.replicas int                Number of ClickHouse Keeper replicas. Value range [1]. (default 1)
      --keeper.storage string              Size of the keeper's persistent storage volume in Gi. Value range [1, 10000].
      --keeper.storage-class-name string   Kubernetes storage class name for keeper persistent volumes.
      --keeper.tolerations stringArray     
      --memory string                      The amount of memory requested for each ClickHouse instance, unit is Gi. Value range [0.5, 1000].
      --mode string                        Define the ClickHouse cluster topology mode. Legal values [standalone, cluster]. (default "cluster")
      --node-labels stringToString         Node label selector (default [])
  -o, --output format                      Prints the output in the specified format. Allowed values: JSON and YAML (default yaml)
      --pod-anti-affinity string           Pod anti-affinity type, one of: (Preferred, Required) (default "Preferred")
      --replicas int                       Number of replicas per shard. Value range [1]. (default 2)
      --service-account.name string        Name of the service account to create or use.
      --service.type string                Type of service to expose ClickHouse. Valid options are 'ExternalName', 'ClusterIP', 'NodePort', and 'LoadBalancer'. Legal values [ExternalName, ClusterIP, NodePort, LoadBalancer]. (default "ClusterIP")
      --sharding                           Enable or disable sharding within the ClickHouse cluster. (default true)
      --shards int                         Number of shards in the ClickHouse cluster. Value range [1]. (default 1)
      --storage string                     Size of the persistent storage volume in Gi. Value range [1, 10000].
      --storage-class-name string          Kubernetes storage class name to use for persistent volumes.
      --tenancy string                     Tenancy options, one of: (SharedNode, DedicatedNode) (default "SharedNode")
      --termination-policy string          The termination policy of cluster. Legal values [DoNotTerminate, Halt, Delete, WipeOut]. (default "Delete")
      --tls.enabled                        Enable or disable TLS for the ClickHouse cluster.
      --tls.issuer string                  Issuer type for TLS certificates. Options are 'KubeBlocks' or 'UserProvided'. Legal values [KubeBlocks, UserProvided]. (default "UserProvided")
      --tls.secret-name string             Name of the secret containing the TLS certificate when using UserProvided issuer. (default "clickhouse-cluster-tls")
      --tolerations strings                Tolerations for cluster, such as "key=value:effect,key:effect", for example '"engineType=mongo:NoSchedule", "diskType:NoSchedule"'
      --topology-keys stringArray          Topology keys for affinity
      --version string                     Engine Version of the ClickHouse cluster. (default "24.8.3")
```

### Options inherited from parent commands

```
      --as string                      Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --as-group stringArray           Group to impersonate for the operation, this flag can be repeated to specify multiple groups.
      --as-uid string                  UID to impersonate for the operation.
      --cache-dir string               Default cache directory (default "$HOME/.kube/cache")
      --certificate-authority string   Path to a cert file for the certificate authority
      --client-certificate string      Path to a client certificate file for TLS
      --client-key string              Path to a client key file for TLS
      --cluster string                 The name of the kubeconfig cluster to use
      --context string                 The name of the kubeconfig context to use
      --disable-compression            If true, opt-out of response compression for all requests to the server
      --insecure-skip-tls-verify       If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string              Path to the kubeconfig file to use for CLI requests.
      --match-server-version           Require server version to match client version
  -n, --namespace string               If present, the namespace scope for this CLI request
      --request-timeout string         The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                  The address and port of the Kubernetes API server
      --tls-server-name string         Server name to use for server certificate validation. If it is not provided, the hostname used to contact the server is used
      --token string                   Bearer token for authentication to the API server
      --user string                    The name of the kubeconfig user to use
```

### SEE ALSO

* [kbcli cluster create](kbcli_cluster_create.md)	 - Create a cluster.

#### Go Back to [CLI Overview](cli.md) Homepage.

