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

## Package Creation

Creating a package for BPM is simple

To create a package you need to
1) Create a working directory
```
mkdir my_bpm_package
```
2) Create a pkg.info file following this format (You can find examples in the test_packages directory)
```
name: my_package
description: My package's description
version: 1.0
architecture: x86_64
type: <binary/source>
depends: dependency1,dependency2
make_depends: make_depend1,make_depend2
```
depends and make depends are optional fields, you may skip them if you'd like
### Binary Packages
3) If you are making a binary package, simply create a 'files' directory
```
mkdir files
```
4) Copy all your binaries along with the directories they reside in (i.e files/usr/bin/my_binary)
5) Either copy the bpm-create script from the bpm-utils test package into your /usr/local/bin directory or install the bpm-utils.bpm package
6) Run the following
```
bpm-create <filename_without_extension>
```
7) It's done! You now hopefully have a working BPM package!
### Source Packages
3) If you are making a source package, you need to create a 'source.sh' file
```
touch source.sh
```
4) You are able to run bash code in this file. BPM will extract this file in a directory under /tmp and it will be ran there
5) Your goal is to download your program's source code with either git, wget, curl, etc. and put the binaries under a folder called 'output' in the root of the temp directory. There is a simple example script with helpful comments in the htop-src test package
6) As of this moment there is no script to automate package compression like for binary packages. You will need to create the archive manually
```
tar -czvf my_package-src.bpm pkg.info source.sh
```
7) That's it! Your source package should now be compiling correctly!