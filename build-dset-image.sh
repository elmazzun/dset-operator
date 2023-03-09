#!/bin/bash

pushd dset-image
docker build . -t elmazzun/dset && docker run -it --rm elmazzun/dset
popd
