#!/bin/bash

set -e

IMAGE_NAME="milan98080/ubuntu:gha-latest"

echo "Building Docker image: $IMAGE_NAME"
docker build -t $IMAGE_NAME .

echo "Pushing to Docker Hub..."
docker push $IMAGE_NAME

echo "Build and push completed successfully!"
echo "Image available at: https://hub.docker.com/r/milan98080/ubuntu"
