---
title: kbcli cluster create mysql
---

Create a mysql cluster.

```
kbcli cluster create mysql NAME [flags]
```

### Examples

```
  # Create a cluster with the default values
  kbcli cluster create mysql
  
  # Create a cluster with the specified cpu, memory and storage
  kbcli cluster create mysql --cpu 1 --memory 2 --storage 10
```

### Options

```
      --cpu float                                                   CPU cores. Value range [0.5, 64]. (default 1)
      --disable-exporter                                            Enable or disable monitor. (default true)
      --dry-run string[="unchanged"]                                Must be "client", or "server". If with client strategy, only print the object that would be sent, and no data is actually sent. If with server strategy, submit the server-side request, but no data is persistent. (default "none")
      --edit                                                        Edit the API resource before creating
  -h, --help                                                        help for mysql
      --memory float                                                Memory, the unit is Gi. Value range [0.5, 1000]. (default 1)
      --node-labels stringToString                                  Node label selector (default [])
      --orchestrator.cluster-service-selector.cluster-name string   orchestrator cluster name for service selector
      --orchestrator.cluster-service-selector.namespace string      orchestrator cluster namespace for service selector
      --orchestrator.service-reference.endpoint string              Endpoint name of the service reference, format: <host>:<port>
  -o, --output format                                               Prints the output in the specified format. Allowed values: JSON and YAML (default yaml)
      --pod-anti-affinity string                                    Pod anti-affinity type, one of: (Preferred, Required) (default "Preferred")
      --proxysql.cpu float                                           (default 1)
      --proxysql.memory float                                       Memory, the unit is Gi. (default 1)
      --proxysql.replicas int                                        (default 1)
      --replicas int                                                The number of replicas. Value range [1, 5]. (default 1)
      --storage float                                               Storage size, the unit is Gi. Value range [1, 10000]. (default 20)
      --tenancy string                                              Tenancy options, one of: (SharedNode, DedicatedNode) (default "SharedNode")
      --termination-policy string                                   The termination policy of cluster. Legal values [DoNotTerminate, Delete, WipeOut]. (default "Delete")
      --tolerations strings                                         Tolerations for cluster, such as "key=value:effect,key:effect", for example '"engineType=mongo:NoSchedule", "diskType:NoSchedule"'
      --topology string                                             Topology type of the serve. Note that under the orc/orc-proxysql topology, it is necessary to specify the Orchestrator cluster information. You should choose either orchestrator.cluster-service-selector or orchestrator.service-reference. This means that depending on your setup, you will configure one of these options to properly integrate with the Orchestrator service for managing your MySQL cluster. Legal values [semisync, semisync-proxysql, mgr, mgr-proxysql, orc, orc-proxysql]. (default "semisync")
      --topology-keys stringArray                                   Topology keys for affinity
      --version string                                              MySQL version Legal values [8.0.39, 8.0.38, 8.0.37, 8.0.36, 8.4.2, 8.4.1, 8.4.0, 5.7.44]. (default "8.0.39")
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

