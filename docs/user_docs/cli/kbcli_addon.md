---
title: kbcli addon
---

Addon command.

### Options

```
  -h, --help   help for addon
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


* [kbcli addon describe](kbcli_addon_describe.md)	 - Describe an addon specification.
* [kbcli addon disable](kbcli_addon_disable.md)	 - Disable an addon.
* [kbcli addon enable](kbcli_addon_enable.md)	 - Enable an addon.
* [kbcli addon index](kbcli_addon_index.md)	 - Manage custom addon indexes
* [kbcli addon install](kbcli_addon_install.md)	 - Install KubeBlocks addon
* [kbcli addon list](kbcli_addon_list.md)	 - List addons.
* [kbcli addon purge](kbcli_addon_purge.md)	 - Purge the sub-resources of specified addon and versions
* [kbcli addon search](kbcli_addon_search.md)	 - Search the addon from index
* [kbcli addon uninstall](kbcli_addon_uninstall.md)	 - Uninstall an existed addon
* [kbcli addon upgrade](kbcli_addon_upgrade.md)	 - Upgrade an existed addon to latest version or a specified version

#### Go Back to [CLI Overview](cli.md) Homepage.

