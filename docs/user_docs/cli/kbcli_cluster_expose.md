---
title: kbcli cluster expose
---

Expose a cluster with a new endpoint, the new endpoint can be found by executing 'kbcli cluster describe NAME'.

```
kbcli cluster expose NAME --enable=[true|false] --type=[intranet|internet] [flags]
```

### Examples

```
  # Expose a cluster to intranet
  kbcli cluster expose mycluster --type intranet --enable=true
  
  # Expose a cluster to public internet
  kbcli cluster expose mycluster --type internet --enable=true
  
  # Stop exposing a cluster
  kbcli cluster expose mycluster --type intranet --enable=false
```

### Options

```
      --auto-approve                   Skip interactive approval before exposing the cluster
      --components strings             Component names to this operations
      --dry-run string[="unchanged"]   Must be "client", or "server". If with client strategy, only print the object that would be sent, and no data is actually sent. If with server strategy, submit the server-side request, but no data is persistent. (default "none")
      --edit                           Edit the API resource before creating
      --enable string                  Enable or disable the expose, values can be true or false
      --force                           skip the pre-checks of the opsRequest to run the opsRequest forcibly
  -h, --help                           help for expose
      --name string                    OpsRequest name. if not specified, it will be randomly generated
  -o, --output format                  Prints the output in the specified format. Allowed values: JSON and YAML (default yaml)
      --role-selector roleSelector     The Component's exposed Services may target replicas based on their roles using roleSelector, this flag must be set when the component specified has roles
      --sub-type string                Expose sub type, currently supported types are 'NodePort', 'LoadBalancer', only available if type is intranet (default "LoadBalancer")
      --ttlSecondsAfterSucceed int     Time to live after the OpsRequest succeed
      --type string                    Expose type, currently supported types are 'intranet', 'internet'
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

* [kbcli cluster](kbcli_cluster.md)	 - Cluster command.

#### Go Back to [CLI Overview](cli.md) Homepage.

