/*
 * Copyright (c) 2025 International Business Machines
 * All rights reserved.
 *
 *  SPDX-License-Identifier: MIT
 *
 * Authors: ndevos@ibm.com
 */

package nvmeof

//go:generate protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative --proto_path=../../../control/proto ../../../control/proto/gateway.proto
