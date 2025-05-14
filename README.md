# Bubble Package Manager (BPM)
## _A simple package manager_

BPM is a simple package manager for Linux systems

## Features
- Simple to use subcommands
- Can install binary packages (and source packages in the future)

## Information
BPM is still in very early development. Do not install it without knowing what you are doing. I would only recommend using it in a Virtual Machine for testing

## Build from source

- Download `go` from your package manager or from the go website
- Download `make` from your package manager
- Download `which` from your package manager
- Run the following command to compile the project. You may need to set the `GO` environment variable if your Go installation is not in your PATH
```sh
make
```
- Run the following command to install BPM into your system. You may also append a DESTDIR variable at the end of this line if you wish to install in a different location
```sh
make install PREFIX=/usr SYSCONFDIR=/etc
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

Package creation is simplified using the bpm-utils package which contains helper scripts for creating and archiving packages

Learn more here: https://git.enumerated.dev/bubble-package-manager/bpm-utils