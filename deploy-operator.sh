#!/bin/bash

# 'source' this file in order to export to your current environment the
# DSET_IMAGE env var (required to tell Operator what image to use) and
# deploy the Operator in your cluster by calling install_operator()

export DSET_IMAGE="quay.io/quay/busybox"

function install_operator {
    kind load docker-image controller:0.1.0 --name taccitua && \
        make generate && \
        make manifests && \
        make deploy
}
