UefiVarMonitor
===============

这是一个示例运行时DXE驱动程序（UEFI驱动程序），通过在C和Rust中挂钩运行时服务表来监控对UEFI变量的访问。

该项目旨在提供一个小型运行时驱动程序的示例。

Rust实现完全是为了作者的学习。

项目概述
------------------

* UefiVarMonitorCore

    这是一个UEFI运行时驱动程序，挂钩`GetVariable`和`SetVariable`运行时服务，并将其使用情况记录到串行输出中。用不到300行C代码编写。

* uefi-var-monitor-rust

    与`UefiVarMonitorCore`几乎等效的Rust实现。

* UefiVarMonitorEnhanced

    `UefiVarMonitorCore`的增强版本，允许Windows驱动程序注册上述运行时服务的内联回调。这也可以用来更改参数并阻止这些调用。

* UefiVarMonitorClient

    注册回调与`UefiVarMonitorEnhanced`的示例Windows驱动程序。

构建
---------

* UefiVarMonitorCore和UefiVarMonitorEnhanced

    1. 设置edk2构建环境
    2. 将`UefiVarMonitorPkg`复制为`edk2\UefiVarMonitorPkg`
    3. 在edk2构建命令提示符下，运行以下命令：
        ```
        > edksetup.bat
        > build -t VS2019 -a X64 -b NOOPT -p UefiVarMonitorPkg\UefiVarMonitorPkg.dsc -D DEBUG_ON_SERIAL_PORT
        ```
       或在Linux或WSL上，
        ```
        $ . edksetup.sh
        $ build -t GCC5 -a X64 -b NOOPT -p UefiVarMonitorPkg/UefiVarMonitorPkg.dsc -D DEBUG_ON_SERIAL_PORT
        ```

* uefi-var-monitor-rust

    1. 安装夜间版本的Rust编译器。以下是在Linux上的示例，但在Windows上大致相同。
        ```
        $ sudo snap install rustup --classic
        $ rustup default nightly
        $ rustup component add rust-src
        ```
    2. 构建项目。
        ```
        $ cd uefi-var-monitor-rust
        $ cargo build
        ```

* UefiVarMonitorClient

    这是一个标准的Windows驱动程序。需要VS2019和10.0.18362或更高版本的WDK。
