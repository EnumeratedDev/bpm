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

1) Make sure you have installed the bpm-utils package from the test-packages directory in the repository
2) Run the following command (You can run the comamnd with no arguments to see available options)
```
bpm-setup -D my_bpm_package -t <binary/source>
```
3) This will create a directory named 'my_bpm_package' under the current directory with all the required files for the chosen package type
4) You are able to edit the pkg.info descriptor file inside the newly created directory to your liking. Here's an example of what a descriptor  file could look like
```
name: my_package
description: My package's description
version: 1.0
architecture: x86_64
url: https://www.my-website.com/ (Optional)
license: MyLicense (Optional)
type: <binary/source>
depends: dependency1,dependency2 (Optional)
make_depends: make_depend1,make_depend2 (Optional)
```
### Binary Packages
3) If you are making a binary package, copy all your binaries along with the directories they reside in (i.e files/usr/bin/my_binary)
6) Run the following to create a package archive
```
bpm-package <filename.bpm>
```
7) It's done! You now hopefully have a working BPM package!
### Source Packages
3) If you would like to bundle patches or other files with your source package place them in the 'source-files' directory. They will be extracted to the same location as the source.sh file during compilation
4) You need to edit your 'source.sh' file, the default source.sh template should explain the basic process of compiling your program
5) Your goal is to download your program's source code with either git, wget, curl, etc. and put the binaries under a folder called 'output' in the root of the temp directory. There is a simple example script with helpful comments in the htop-src test package
6) When you are done making your source.sh script run the following to create a package archive
```
bpm-package <filename.bpm>
```
7) That's it! Your source package should now be compiling correctly!