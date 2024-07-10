# Bubble Package Manager (BPM)
## _A simple package manager_

BPM is a simple package manager for Linux systems

## Features
- Simple to use subcommands
- Can install binary and source packages
- Can be easily installed on practically any system
- No bloat

## Information

BPM is still in very early development. It should not be installed on any system you use seriously. I recommend trying this out in a VM or container. In addition to this, this is one of the first projects I have made using the go programming language so code quality may not be the best. This project was made to help me learn go and how linux systems work better. It is not meant to replace the big package managers in any way

## Build from source

- Download `go` from your package manager or from the go website
- Download `make` from your package manager
- Run the following command to compile the project
```
make
```
- Run the following command to install stormfetch into your system. You may also append a DESTDIR variable at the end of this line if you wish to install in a different location
```
make install PREFIX=/usr SYSCONFDIR=/etc
```

## How to use

You are able to install bpm packages by typing the following:
```sh
bpm install /path/to/package.bpm
```
You may also use the -y flag as shown below to bypass the installation confirmation prompt
```sh
bpm install -y /path/to/package.bpm
```
Flags must strictly be typed before the first package path otherwise they'll be read as package locations themselves

You can remove an installed package by typing the following
```sh
bpm remove package_name
```
The -y flag applies here as well if you wish to skip the removal confirmation prompt

For information on the rest of the commands simply use the help command or pass in no arguments at all
```
bpm help
```

## Package Creation

Package creation is simplified using the bpm-utils package which contains helper scripts for creating and archiving packages. \
Learn more here: https://gitlab.com/bubble-package-manager/bpm-utils