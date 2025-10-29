#!/bin/bash
set -e

apt-get clean
apt-get update
apt-get install -y stellar-horizon=$PACKAGE_VERSION

mkdir released
cd released

wget https://github.com/stellar/go/releases/download/$TAG/$TAG-darwin-amd64.tar.gz
wget https://github.com/stellar/go/releases/download/$TAG/$TAG-linux-amd64.tar.gz
wget https://github.com/stellar/go/releases/download/$TAG/$TAG-linux-arm.tar.gz
wget https://github.com/stellar/go/releases/download/$TAG/$TAG-windows-amd64.zip

tar -xvf $TAG-darwin-amd64.tar.gz
tar -xvf $TAG-linux-amd64.tar.gz
tar -xvf $TAG-linux-arm.tar.gz
unzip $TAG-windows-amd64.zip

cd -

# Since Go 1.18 vcs (git) info is added to the binary. One of the values is:
# vcs.modified which determines if git working dir is clean. We need to
# specifically add the files below to .gitignore to make git ignore them.
touch ~/.gitignore
echo -e "check.sh\n" >> ~/.gitignore
echo -e "released/\n" >> ~/.gitignore
git config --global core.excludesFile '~/.gitignore'

git pull origin --tags
git checkout $TAG
# -keep: artifact directories are not removed after packaging
CIRCLE_TAG=$TAG go run -v ./support/scripts/build_release_artifacts -keep

echo "RESULTS"
echo "======="
echo ""
echo "compiled version"
./dist/$TAG-linux-amd64/horizon version

echo "github releases version"
./released/$TAG-linux-amd64/horizon version

echo "debian package version"
stellar-horizon version

echo ""

suffixes=(darwin-amd64 linux-amd64 linux-arm windows-amd64)
for S in "${suffixes[@]}"
do
    released=""
    dist=""
    msg=""
    
    if [ -f "./released/$TAG-$S.tar.gz" ]; then
        released=($(shasum -a 256 ./released/$TAG-$S/horizon))
    else
        # windows
        released=($(shasum -a 256 ./released/$TAG-$S/horizon.exe))
    fi

    if [ -f "./dist/$TAG-$S.tar.gz" ]; then
        dist=($(shasum -a 256 ./dist/$TAG-$S/horizon))
    else
        # windows
        dist=($(shasum -a 256 ./dist/$TAG-$S/horizon.exe))
    fi

    if [ $S == "linux-amd64" ]; then
        path=$(which stellar-horizon)
        debian=($(shasum -a 256 $path))

        if [[ "$released" == "$dist" && "$dist" == "$debian" ]]; then
            msg="$TAG-$S ok"
        else
            msg="$TAG-$S NO MATCH! github=$released compile=$dist debian=$debian"
        fi
    else
        if [ "$released" == "$dist" ]; then
            msg="$TAG-$S ok"
        else
            msg="$TAG-$S NO MATCH! github=$released compile=$dist"
        fi
    fi

    echo $msg
done