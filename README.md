# Bubble Package Manager (BPM)
## _The official package manager for Tide Linux_

## Features
- Simple to use subcommands
- Can install both binary and source packages

## Information
BPM is still in very early development. There may be bugs that could completely break installations. For now it is recommended you use this only in Virtual Machines, containers or in a Tide Linux installation

## Build from source

- Download `go` from your package manager or from the go website
- Download `make` from your package manager
- Run the following command to compile the project. You may need to set the `GO` environment variable if your Go installation is not in your PATH
```sh
make
```
- Run the following command to install BPM into your system. You may also append a DESTDIR variable at the end of this line if you wish to install in a different location
```sh
make install PREFIX=/usr SYSCONFDIR=/etc
make install-config PREFIX=/usr SYSCONFDIR=/etc
```

## How to use

You are able to install bpm packages by typing the following:
```sh
bpm install /path/to/package.bpm
```
You can also use the package name directly if using databases
```sh
bpm install package_name
```
The -y flag may be used as shown below to bypass the confirmation prompt
```sh
bpm install -y /path/to/package.bpm
```
Flags must strictly be typed before the first package path or name, otherwise they'll be read as package locations themselves

You can remove an installed package by typing the following
```sh
bpm remove package_name
```

To remove all unused dependencies and clean cached files try using the cleanup command
```sh
bpm cleanup
```

If using databases, all packages can be updated using this simple command
```sh
bpm update
```

For information on the rest of the commands simply use the help command or pass in no arguments at all
```sh
bpm help
```

## Package Creation

Package creation is simplified using the bpm-utils package which contains helper scripts for creating packages

Learn more here: https://git.enumerated.dev/bubble-package-manager/bpm-utils
