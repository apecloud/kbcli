---
title: kbcli addon upgrade
---

Upgrade an existed addon to latest version or a specified version

```
kbcli addon upgrade [flags]
```

### Examples

```
  # upgrade an addon from default index to latest version
  kbcli addon upgrade apecloud-mysql
  
  # upgrade an addon from default index to latest version and skip KubeBlocks version compatibility check
  kbcli addon upgrade apecloud-mysql --force
  
  # upgrade an addon to latest version from a specified index
  kbcli addon upgrade apecloud-mysql --index my-index
  
  # upgrade an addon with a specified version default index
  kbcli addon upgrade apecloud-mysql --version 0.7.0
  
  # upgrade an addon with a specified version, default index and a different version of cluster chart
  kbcli addon upgrade apecloud-mysql --version 0.7.0 --cluster-chart-version 0.7.1
  
  # non-inplace upgrade an addon with a specified version
  kbcli addon upgrade apecloud-mysql  --inplace=false --version 0.7.0
  
  # non-inplace upgrade an addon with a specified addon name
  kbcli addon upgrade apecloud-mysql --inplace=false --name apecloud-mysql-0.7.0
```

### Options

```
      --cluster-chart-repo string      specify the repo of cluster chart, use the url of 'kubeblocks-addons' by default (default "https://jihulab.com/api/v4/projects/150246/packages/helm/stable")
      --cluster-chart-version string   specify the cluster chart version, use the same version as the addon by default
      --force                          force upgrade the addon and ignore the version check
  -h, --help                           help for upgrade
      --index string                   specify the addon index index, use 'kubeblocks' by default (default "kubeblocks")
      --inplace                        when inplace is false, it will retain the existing addon and reinstall the new version of the addon, otherwise the upgrade will be in-place. The default is true. (default true)
      --name string                    name is the new version addon name need to set by user when inplace is false, it also will be used as resourceNamePrefix of an addon with multiple version.
      --path string                    specify the local path contains addon CRs and needs to be specified when operating offline
      --version string                 specify the addon version
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

* [kbcli addon](kbcli_addon.md)	 - Addon command.

#### Go Back to [CLI Overview](cli.md) Homepage.

