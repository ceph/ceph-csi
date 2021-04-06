/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Reference
// https://github.com/kubernetes-csi/csi-lib-utils/blob/master/protosanitizer/protosanitizer.go

// Package protosanitizer supports logging of gRPC messages without accidentally
// revealing sensitive fields.
package protosanitizer

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/golang/protobuf/descriptor"
	"github.com/golang/protobuf/proto"
	protobuf "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"google.golang.org/protobuf/types/descriptorpb"
)

// StripReplicationSecrets returns a wrapper around the original Replication gRPC message
// which has a Stringer implementation that serializes the message
// as one-line JSON, but without including secret information.
// Instead of the secret value(s), the string "***stripped***" is
// included in the result.
//
// StripReplicationSecrets relies on an extension in Replication and thus can only
// be used for messages based on that or a more recent spec!
//
// StripReplicationSecrets itself is fast and therefore it is cheap to pass the
// result to logging functions which may or may not end up serializing
// the parameter depending on the current log level.
func StripReplicationSecrets(msg interface{}) fmt.Stringer {
	return &stripSecrets{msg, isReplicationSecret}
}

type stripSecrets struct {
	msg interface{}

	isSecretField func(field *protobuf.FieldDescriptorProto) bool
}

func (s *stripSecrets) String() string {
	// First convert to a generic representation. That's less efficient
	// than using reflect directly, but easier to work with.
	var parsed interface{}
	b, err := json.Marshal(s.msg)
	if err != nil {
		return fmt.Sprintf("<<json.Marshal %T: %s>>", s.msg, err)
	}
	if err := json.Unmarshal(b, &parsed); err != nil {
		return fmt.Sprintf("<<json.Unmarshal %T: %s>>", s.msg, err)
	}

	// Now remove secrets from the generic representation of the message.
	s.strip(parsed, s.msg)

	// Re-encoded the stripped representation and return that.
	b, err = json.Marshal(parsed)
	if err != nil {
		return fmt.Sprintf("<<json.Marshal %T: %s>>", s.msg, err)
	}
	return string(b)
}

func (s *stripSecrets) strip(parsed, msg interface{}) {
	protobufMsg, ok := msg.(descriptor.Message)
	if !ok {
		// Not a protobuf message, so we are done.
		return
	}

	// The corresponding map in the parsed JSON representation.
	parsedFields, ok := parsed.(map[string]interface{})
	if !ok {
		// Probably nil.
		return
	}

	// Walk through all fields and replace those with ***stripped*** that
	// are marked as secret. This relies on protobuf adding "json:" tags
	// on each field where the name matches the field name in the protobuf
	_, md := descriptor.ForMessage(protobufMsg)
	fields := md.GetField()
	for _, field := range fields {
		if s.isSecretField(field) {
			// Overwrite only if already set.
			if _, ok := parsedFields[field.GetName()]; ok {
				parsedFields[field.GetName()] = "***stripped***"
			}
		} else if field.GetType() == protobuf.FieldDescriptorProto_TYPE_MESSAGE {
			typeName := strings.TrimPrefix(field.GetTypeName(), ".")
			t := proto.MessageType(typeName)
			if t == nil || t.Kind() != reflect.Ptr {
				// Shouldn't happen, but
				// better check anyway instead
				// of panicking.
				continue
			}
			v := reflect.New(t.Elem())

			// Recursively strip the message(s) that
			// the field contains.
			i := v.Interface()
			entry := parsedFields[field.GetName()]
			if slice, ok := entry.([]interface{}); ok {
				for _, entry := range slice {
					s.strip(entry, i)
				}
			} else {
				// Single value.
				s.strip(entry, i)
			}
		}
	}
}

// isReplicationSecret uses the replication.file_replication_proto_extTypes extension from replication to
// determine whether a field contains secrets.
func isReplicationSecret(field *protobuf.FieldDescriptorProto) bool {
	ex, err := proto.GetExtension(field.Options, eReplicationSecret)
	return err == nil && ex != nil && *ex.(*bool)
}

// eReplicationSecret represents the secret format at
// https://github.com/csi-addons/spec/blob/v0.1.0/lib/go/replication/
// replication.pb.go#L640-L649 .
var eReplicationSecret = &proto.ExtensionDesc{
	ExtendedType:  (*descriptorpb.FieldOptions)(nil),
	ExtensionType: (*bool)(nil),
	Field:         1099,
	Name:          "replication.replication_secret",
	Tag:           "varint,1099,opt,name=replication_secret",
	Filename:      "replication.proto",
}
