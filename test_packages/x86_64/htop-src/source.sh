# This file is read and executed by BPM to compile htop. It will run inside a temporary folder in /tmp during execution
echo "Compiling htop..."
# Creating 'source' directory
mkdir source
# Cloning the git repository into the 'source' directory
git clone https://github.com/htop-dev/htop.git source
# Changing directory into 'source'
cd source
# Configuring and making htop according to the installation instructions in the repository
./autogen.sh
./configure --prefix=/usr
make
# Creating an 'output' directory in the root of the temporary directory created by BPM
mkdir ./../output/
# Setting $dir to the 'output' directory
dir=$(pwd)/../output/
# Installing htop to $dir
make DESTDIR="$dir" install
# The compilation is done. BPM will now copy the files from the 'output' directory into the root of your system
echo "htop compilation complete!"
