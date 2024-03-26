# Bubble Package Manager (BPM)
## _A simple package manager_

BPM is a simple package manager for Linux systems

## Features
- Simple to use subcommands
- Can install binary and source packages (source packages are still under development)
- Can be easily installed on practically any system
- No bloat

## Information

BPM is still in very early development. It should not be installed on any system you use seriously. I recommend trying this out in a VM or container. In addition to this, this is one of the first projects I have made using the go programming language so code quality may not be the best. This project was made to help me learn go and how linux systems work better. It is not meant to replace the big package managers in any way

## Build from source

BPM requires go 1.22 or above to be built properly

```sh
git clone https://gitlab.com/bubble-package-manager/bpm.git
cd bpm
mkdir build
go build -o ./build/bpm capcreepergr.me/bpm
```
You are now able to copy the executable in the ./build directory in a VM or container's /usr/bin/ directory

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