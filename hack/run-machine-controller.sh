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

set -e

: "${DEBUG:="false"}"

# Use a special env variable for machine-controller only
# This kubeconfig should point to the cluster where machinedeployments, machines are installed.
MC_KUBECONFIG=${MC_KUBECONFIG:-$(dirname $0)/../.kubeconfig}
# If you want to use the default kubeconfig `export MC_KUBECONFIG=$KUBECONFIG`

# `-use-osm` flag can be specified if https://github.com/kubermatic/operating-system-manager is used to manage user data.

if [[ "${DEBUG}" == "true" ]]; then
    GOTOOLFLAGS="-v -gcflags='all=-N -l'" LDFLAGS="" make -C $(dirname $0)/.. build-machine-controller
    RUNCMD="dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec $(dirname $0)/../machine-controller --"
else
    make -C $(dirname $0)/.. build-machine-controller
    RUNCMD="$(dirname $0)/../machine-controller"
fi

unset $(set | egrep '^OS_' | sed 's/=.*//')
export $(KUBECONFIG=/tmp/kc kubectl view-secret cloud-env -n kube-system -a)
#. "$(dirname $0)/openrc-dev-fes-admin"

$RUNCMD \
  -kubeconfig=/tmp/kc \
  -worker-count=1 \
  -logtostderr \
  -v=6 \
  -cluster-dns=10.96.0.10 \
  -node-csr-approver \
  -enable-profiling \
  -metrics-address=0.0.0.0:8080 \
  -health-probe-address=0.0.0.0:8085 \
  -node-container-runtime=containerd
