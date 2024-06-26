#!/usr/bin/env bash
# Copyright 2019 The Machine Controller Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

UBUNTU_IMAGE_NAME=${UBUNTU_IMAGE_NAME:-"machine-controller-ubuntu"}

echo "Downloading Ubuntu 18.04 image from upstream..."
curl -L -o ubuntu.img http://cloud-images.ubuntu.com/bionic/current/bionic-server-cloudimg-amd64.img
echo "Uploading Ubuntu 18.04 image to OpenStack..."
openstack image create \
  --container-format bare \
  --disk-format qcow2 \
  --file ubuntu.img \
  ${UBUNTU_IMAGE_NAME}
rm ubuntu.img
echo "Successfully uploaded ${UBUNTU_IMAGE_NAME} to OpenStack..."
