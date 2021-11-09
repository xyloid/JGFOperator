#!/bin/bash

make docker-build
make docker-push-local
make deploy-local

kubectl get pods -n jgfoperator-system

echo ""
echo "jgfoperator podname:" $(kubectl get pods -n jgfoperator-system -o jsonpath="{.items[0].metadata.name}")

# command to get the log 
# kubectl logs -n jgfoperator-system $(kubectl get pods -n jgfoperator-system -o jsonpath="{.items[0].metadata.name}") manager