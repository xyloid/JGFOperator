#!/bin/bash

kubectl logs -n jgfoperator-system $(kubectl get pods -n jgfoperator-system -o jsonpath="{.items[0].metadata.name}") manager