---
title: kbcli cluster create etcd
---

Create a etcd cluster.

```
kbcli cluster create etcd NAME [flags]
```

### Examples

```
  # Create a cluster with the default values
  kbcli cluster create etcd
  
  # Create a cluster with the specified cpu, memory and storage
  kbcli cluster create etcd --cpu 1 --memory 2 --storage 10
```

### Options

```
      --availability-policy string                   The availability policy of cluster. Legal values [none, node, zone]. (default "node")
      --client-service.name string                   When name is not empty, clientService will be enabled
      --client-service.node-port int                 Optional nodePort if clientService type is NodePort
      --client-service.port int                      Port for clientService (default 2379)
      --client-service.role string                   Role for clientService (default "leader")
      --client-service.type string                   Etcd service type, valid options are ExternalName, ClusterIP, NodePort, and LoadBalancer (default "ClusterIP")
      --disable-exporter                             Enable or disable monitor. (default true)
      --fullname-override string                     Override the full name of the chart
  -h, --help                                         help for etcd
      --host-network-accessible                      Specify whether the cluster can be accessed from within the VPC.
      --name-override string                         Override the name of the chart
      --peer-service.name string                     When name is not empty, peerService will be enabled
      --peer-service.type string                     Etcd peerService type, recommended option is LoadBalancer (default "LoadBalancer")
      --persistence.data.size string                 Size of data volume (default "1Gi")
      --persistence.data.storage-class-name string   Storage class of backing PVC
      --persistence.enabled                          Enable persistence using Persistent Volume Claims (default true)
      --publicly-accessible                          Specify whether the cluster can be accessed from the public internet.
      --rbac-enabled                                 Specify whether rbac resources will be created by client, otherwise KubeBlocks server will try to create rbac resources.
      --replicas int                                 Number of replicas (default 3)
      --tenancy string                               The tenancy of cluster. Legal values [SharedNode, DedicatedNode]. (default "SharedNode")
      --termination-policy string                    The termination policy of cluster. Legal values [DoNotTerminate, Halt, Delete, WipeOut]. (default "Delete")
      --tls-enable                                   Enable TLS for etcd cluster
      --topology-spread-constraints stringArray      
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
      --dry-run string[="unchanged"]   Must be "client", or "server". If with client strategy, only print the object that would be sent, and no data is actually sent. If with server strategy, submit the server-side request, but no data is persistent. (default "none")
      --edit                           Edit the API resource before creating
      --insecure-skip-tls-verify       If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string              Path to the kubeconfig file to use for CLI requests.
      --match-server-version           Require server version to match client version
  -n, --namespace string               If present, the namespace scope for this CLI request
  -o, --output format                  Prints the output in the specified format. Allowed values: JSON and YAML (default yaml)
      --request-timeout string         The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                  The address and port of the Kubernetes API server
      --tls-server-name string         Server name to use for server certificate validation. If it is not provided, the hostname used to contact the server is used
      --token string                   Bearer token for authentication to the API server
      --user string                    The name of the kubeconfig user to use
```

### SEE ALSO

* [kbcli cluster create](kbcli_cluster_create.md)	 - Create a cluster.

#### Go Back to [CLI Overview](cli.md) Homepage.
