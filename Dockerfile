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

ARG GO_VERSION=1.21.5
FROM docker.io/golang:${GO_VERSION} AS builder
WORKDIR /go/src/github.com/kubermatic/machine-controller
COPY . .
RUN make all

FROM alpine:3.17

RUN apk add --no-cache ca-certificates cdrkit

COPY --from=builder \
    /go/src/github.com/kubermatic/machine-controller/machine-controller \
    /go/src/github.com/kubermatic/machine-controller/machine-controller-userdata-* \
    /go/src/github.com/kubermatic/machine-controller/webhook \
    /usr/local/bin/
USER nobody
