# kbcli

kbcli is a command line interface (CLI) tool for [KubeBlocks](https://github.com/apecloud/kubeblocks). kbcli has the following features:

- Manage KubeBlocks, including installation, uninstallation, viewing status, and upgrading, etc.
- Manage clusters, including creating, deleting, configuration changes, backups and restores, etc.
- Support playgrounds to quickly experience KubeBlocks locally or on the cloud.

## Install kbcli

### Install kbcli on Linux

#### Install with curl

Install the latest linux kbcli to `/usr/local/bin`

```bash
curl -fsSL https://kubeblocks.io/installer/install_cli.sh | bash
```

#### Install using native package management

**Debian-based distributions**

TODO

**Red Hat-based distributions**

TODO

### Install kbcli on macOS

#### Install with curl

Install the latest darwin kbcli to `/usr/local/bin`.

```bash
curl -fsSL https://kubeblocks.io/installer/install_cli.sh | bash
```

#### Install with Homebrew

```bash
brew tap apecloud/tap
brew install kbcli
```

### Install kbcli on Windows

#### Install with powershell

1. Run PowerShell as an administrator and execute `Set-ExecutionPolicy Unrestricted`.
2. Run following script to automatically install kbcli at `C:\Program Files\kbcli-windows-amd64`.

```powershell
powershell -Command " & ([scriptblock]::Create((iwr https://www.kubeblocks.io/installer/install_cli.ps1)))"
```

#### Install with winGet

1. Make sure your `powershell/CMD` support `winget` and run `winget install kbcli`.

#### Install with scoop

1. Add the scoop bucket in `powershell/CMD`:

   - `scoop bucket add scoop-bucket git@github.com:apecloud/scoop-bucket.git ` 
   
   or
   - `scoop bucket add scoop-bucket https://github.com/apecloud/scoop-bucket.git`
2. install the kbcli

    `scoop install kbcli`

#### Install with chocolatey

TODO

### Install using the Binary Releases

Each release of kbcli includes various OSes and architectures. These binary versions can be manually downloaded and installed.

1. Download your desired [kbcli](https://github.com/apecloud/kbcli/releases)
2. Unpack it (e.g. kbcli-linux-amd64-v0.5.2.tar.gz, kbcli-darwin-amd64-v0.5.2.tar.gz)
3. Move it to your desired location.
   * For Linux/macOS - `/usr/local/bin` or any other directory in your $PATH
   * For Windows, create a directory and add it to you System PATH. We recommend creating a `C:\Program Files\kbcli` directory and adding it to the PATH.

## Install KubeBlocks on your local machine

This guide walks you through the quickest way to get started with KubeBlocks, demonstrating how to create a demo environment (Playground) with one kbcli command.

### Prerequisites

Meet the following requirements for a smooth user experience:

* Minimum system requirements:
    * CPU: 4 cores, use `sysctl hw.physicalcpu` command to check CPU;
    * RAM: 4 GB, use `top -d` command to check memory.

* Make sure the following tools are installed on your laptop:
    * [Docker](https://docs.docker.com/get-docker/): v20.10.5 (runc ≥ v1.0.0-rc93) or above;
    * [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl): it is used to interact with Kubernetes clusters;

### Initialize Playground

1. Initialize Playground.

    ```bash
    kbcli playground init
    ```

    This command:
    1. Creates a Kubernetes cluster in the container with [K3d](https://k3d.io/v5.4.6/).
    2. Deploys KubeBlocks in the K3d cluster.
    3. Creates a standalone MySQL cluster.

    > NOTE: If you previously ran `kbcli playground init` and it failed, running it again may cause errors. Please run the `kbcli playground destroy` command first to clean up the environment, then run `kbcli playground init` again.

2. Check the MySQL cluster repeatedly until the status becomes `Running`.

   ```bash
   kbcli cluster list
   ```

### Try KubeBlocks with Playground

1. View the database cluster list.

    ```bash
    kbcli cluster list
    ```

2. View the details of a specified database cluster, such as `STATUS`, `Endpoints`, `Topology`, `Images`, and `Events`.

    ```bash
    kbcli cluster describe mycluster
    ```

3. Access the database cluster.

    Wait until the status of this cluster is `Running`, then run `kbcli cluster connect` to access a specified database cluster. For example,

    ```bash
    kbcli cluster connect mycluster
    ```

4. List and open the grafana dashboard.

    ```bash
    # list all dashboard
    kbcli dashboard list

    # open grafana dashboard
    kbcli dashboard open kubeblocks-grafana
    ```

### Destroy Playground

    ```bash
    kbcli playground destroy
    ```

## Reference Documentation

See the [Reference Documentation](https://kubeblocks.io/docs/preview/user_docs/cli) for more information about kbcli commands.

## Contributing to kbcli

See the [Development Guide](https://github.com/apecloud/kubeblocks/blob/main/docs/DEVELOPING.md) to get started with building and developing.

## Code of Conduct

Please refer to our [KubeBlocks Code of Conduct](https://github.com/apecloud/kubeblocks/blob/main/CODE_OF_CONDUCT.md)
