# kbcli

kbcli is a command line interface (CLI) tool for [KubeBlocks](https://github.com/apecloud/kubeblocks).

kbcli has the following features:
- Manage KubeBlocks, including installation, uninstallation, viewing status, and upgrading, etc.
- Manage clusters, including creating, deleting, configuration changes, backups and restores, etc.
- Support playgrounds to quickly experience KubeBlocks locally or on the cloud.

## What is KubeBlocks

KubeBlocks is an open-source, cloud-native data infrastructure designed to help application developers and platform engineers manage database and analytical workloads on Kubernetes. It is cloud-neutral with multiple cloud service providers supported, offering a unified and declarative approach to increase productivity in DevOps practices.

The name KubeBlocks is derived from Kubernetes and LEGO blocks, which indicates that building database and analytical workloads on Kubernetes can be both productive and enjoyable, like playing with construction toys. KubeBlocks combines the large-scale production experiences of top cloud service providers with enhanced usability and stability.

### Why you need KubeBlocks

Kubernetes has become the de facto standard for container orchestration. It manages an ever-increasing number of stateless workloads with the scalability and availability provided by ReplicaSet and the rollout and rollback capabilities provided by Deployment. However, managing stateful workloads poses great challenges for Kubernetes. Although StatefulSet provides stable persistent storage and unique network identifiers, these abilities are far from enough for complex stateful workloads.

To address these challenges, and solve the problem of complexity, KubeBlocks introduces ReplicationSet and ConsensusSet, with the following capabilities:

- Role-based update order reduces downtime caused by upgrading versions, scaling, and rebooting.
- Maintains the status of data replication and automatically repairs replication errors or delays.

### Goals

- Enhance stateful workloads on Kubernetes, being open-source and cloud-neutral.
- Manage data infrastructure without a high cognitive load of cloud computing, Kubernetes, and database knowledge.
- Reduce costs by only paying for the infrastructure and increasing the utilization of resources with flexible scheduling.
- Support the most popular RDBMS, NoSQL, streaming and analytical systems, and their bundled tools.
- Provide the most advanced user experience based on the concepts of IaC and GitOps.

### Key features

- Be compatible with AWS, GCP, Azure, and Alibaba Cloud.
- Supports MySQL, PostgreSQL, Redis, MongoDB, Kafka, and more.
- Provides production-level performance, resilience, scalability, and observability.
- Simplifies day-2 operations, such as upgrading, scaling, monitoring, backup, and restore.
- Contains a powerful and intuitive command line tool.
- Sets up a full-stack, production-ready data infrastructure in minutes.

## Install kbcli

### Install kbcli on Linux

#### Install with curl

Install the latest linux kbcli to `/usr/local/bin`

```bash
curl -fsSL https://kubeblocks.io/installer/install_cli.sh | bash
```

#### Install using native package management

**Debian-based distributions**

1. Update the apt package index and install packages needed to use the kbcli apt repository:
    ```bash
    sudo apt-get update
    sudo apt-get install curl
    ```
2. Download the kbcli public signing key:
    ```bash
    curl -fsSL https://apecloud.github.io/kbcli-apt/public.key | sudo apt-key add -
    ```
3. Add the kbcli apt repository:
    ```bash
    echo "deb [arch=amd64,arm64] https://apecloud.github.io/kbcli-apt/repo stable main" | sudo tee /etc/apt/sources.list.d/kbcli.list
    ```
4. update apt package index with the new repository and install kbcli:
    ```bash
    sudo apt-get update
    sudo apt-get install kbcli

***For Debian-based distributions, in addition to the installation method above, you can also install it through the following methods:***

1. 
    ```bash
    echo "deb [trusted=yes] https://apt.fury.io/huyongqii/ /" | sudo tee /etc/apt/sources.list.d/kbcli.list
    ```
2. 
    ```bash
    sudo apt update
    ```
3. 
    ```bash
    sudo apt install kbcli
    ```


**Red Hat-based distributions**

1. Installs the package yum-utils using the package manager yum.
    ```bash
    sudo yum install -y yum-utils
    ```
2. Add a new repository to the YUM configuration using yum-config-manager
    ```bash
    sudo yum-config-manager --add-repo https://yum.fury.io/huyongqii/
    ```
3. Update the local cache of available packages and metadata from all configured YUM repositories.
    ```bash
    sudo yum makecache
    ```
4. Install the kbcli.
    ```bash
    sudo yum install kbcli --nogpgcheck
    ```
5. Install the specified version of kbcli.
    ```bash
    sudo yum install kbcli-0.6.0~beta20-1 --nogpgcheck
```
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

Make sure your `powershell/CMD` support `winget` and run:

```bash
winget install kbcli
```

#### Install with scoop

```bash
scoop bucket add scoop-bucket https://github.com/apecloud/scoop-bucket.git
scoop install kbcli
```

#### Install with chocolatey

TODO

### Install using the Binary Releases

Each release of kbcli includes various OSes and architectures. These binary versions can be manually downloaded and installed.

1. Download your desired [kbcli version](https://github.com/apecloud/kbcli/releases)
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

 ```bash
 kbcli playground init
 ```

 This command:
 1. Creates a Kubernetes cluster in the container with [K3d](https://k3d.io/v5.4.6/).
 2. Deploys KubeBlocks in the K3d cluster.
 3. Creates a standalone MySQL cluster.

 > NOTE: If you previously ran `kbcli playground init` and it failed, running it again may cause errors. Please run the `kbcli playground destroy` command first to clean up the environment, then run `kbcli playground init` again.

Check the MySQL cluster repeatedly until the status becomes `Running`.

```bash
kbcli cluster list
```

### Try KubeBlocks with Playground

View the database cluster list.

 ```bash
 kbcli cluster list
 ```

View the details of a specified database cluster, such as `STATUS`, `Endpoints`, `Topology`, `Images`.

 ```bash
 kbcli cluster describe mycluster
 ```

Wait until the status of this cluster is `Running`, run `kbcli cluster connect` to access a specified database cluster. For example,

 ```bash
 kbcli cluster connect mycluster
 ```

List and open the grafana dashboard.

 ```bash
 # list all dashboards
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

## License

kbcli is under the GNU Affero General Public License v3.0.
See the [LICENSE](./LICENSE) file for details.
