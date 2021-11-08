#!/bin/bash

make docker-build
make docker-push-local
make deploy-local