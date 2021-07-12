// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google.golang.org/appengine/internal/urlfetch/urlfetch_service.proto

package urlfetch

import (
	proto "github.com/golang/protobuf/proto"
	fmt "fmt"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = proto.Marshal
	_ = fmt.Errorf
	_ = math.Inf
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

type URLFetchServiceError_ErrorCode int32

const (
	URLFetchServiceError_OK                       URLFetchServiceError_ErrorCode = 0
	URLFetchServiceError_INVALID_URL              URLFetchServiceError_ErrorCode = 1
	URLFetchServiceError_FETCH_ERROR              URLFetchServiceError_ErrorCode = 2
	URLFetchServiceError_UNSPECIFIED_ERROR        URLFetchServiceError_ErrorCode = 3
	URLFetchServiceError_RESPONSE_TOO_LARGE       URLFetchServiceError_ErrorCode = 4
	URLFetchServiceError_DEADLINE_EXCEEDED        URLFetchServiceError_ErrorCode = 5
	URLFetchServiceError_SSL_CERTIFICATE_ERROR    URLFetchServiceError_ErrorCode = 6
	URLFetchServiceError_DNS_ERROR                URLFetchServiceError_ErrorCode = 7
	URLFetchServiceError_CLOSED                   URLFetchServiceError_ErrorCode = 8
	URLFetchServiceError_INTERNAL_TRANSIENT_ERROR URLFetchServiceError_ErrorCode = 9
	URLFetchServiceError_TOO_MANY_REDIRECTS       URLFetchServiceError_ErrorCode = 10
	URLFetchServiceError_MALFORMED_REPLY          URLFetchServiceError_ErrorCode = 11
	URLFetchServiceError_CONNECTION_ERROR         URLFetchServiceError_ErrorCode = 12
)

var URLFetchServiceError_ErrorCode_name = map[int32]string{
	0:  "OK",
	1:  "INVALID_URL",
	2:  "FETCH_ERROR",
	3:  "UNSPECIFIED_ERROR",
	4:  "RESPONSE_TOO_LARGE",
	5:  "DEADLINE_EXCEEDED",
	6:  "SSL_CERTIFICATE_ERROR",
	7:  "DNS_ERROR",
	8:  "CLOSED",
	9:  "INTERNAL_TRANSIENT_ERROR",
	10: "TOO_MANY_REDIRECTS",
	11: "MALFORMED_REPLY",
	12: "CONNECTION_ERROR",
}

var URLFetchServiceError_ErrorCode_value = map[string]int32{
	"OK":                       0,
	"INVALID_URL":              1,
	"FETCH_ERROR":              2,
	"UNSPECIFIED_ERROR":        3,
	"RESPONSE_TOO_LARGE":       4,
	"DEADLINE_EXCEEDED":        5,
	"SSL_CERTIFICATE_ERROR":    6,
	"DNS_ERROR":                7,
	"CLOSED":                   8,
	"INTERNAL_TRANSIENT_ERROR": 9,
	"TOO_MANY_REDIRECTS":       10,
	"MALFORMED_REPLY":          11,
	"CONNECTION_ERROR":         12,
}

func (x URLFetchServiceError_ErrorCode) Enum() *URLFetchServiceError_ErrorCode {
	p := new(URLFetchServiceError_ErrorCode)
	*p = x
	return p
}

func (x URLFetchServiceError_ErrorCode) String() string {
	return proto.EnumName(URLFetchServiceError_ErrorCode_name, int32(x))
}

func (x *URLFetchServiceError_ErrorCode) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(URLFetchServiceError_ErrorCode_value, data, "URLFetchServiceError_ErrorCode")
	if err != nil {
		return err
	}
	*x = URLFetchServiceError_ErrorCode(value)
	return nil
}

func (URLFetchServiceError_ErrorCode) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_urlfetch_service_b245a7065f33bced, []int{0, 0}
}

type URLFetchRequest_RequestMethod int32

const (
	URLFetchRequest_GET    URLFetchRequest_RequestMethod = 1
	URLFetchRequest_POST   URLFetchRequest_RequestMethod = 2
	URLFetchRequest_HEAD   URLFetchRequest_RequestMethod = 3
	URLFetchRequest_PUT    URLFetchRequest_RequestMethod = 4
	URLFetchRequest_DELETE URLFetchRequest_RequestMethod = 5
	URLFetchRequest_PATCH  URLFetchRequest_RequestMethod = 6
)

var URLFetchRequest_RequestMethod_name = map[int32]string{
	1: "GET",
	2: "POST",
	3: "HEAD",
	4: "PUT",
	5: "DELETE",
	6: "PATCH",
}

var URLFetchRequest_RequestMethod_value = map[string]int32{
	"GET":    1,
	"POST":   2,
	"HEAD":   3,
	"PUT":    4,
	"DELETE": 5,
	"PATCH":  6,
}

func (x URLFetchRequest_RequestMethod) Enum() *URLFetchRequest_RequestMethod {
	p := new(URLFetchRequest_RequestMethod)
	*p = x
	return p
}

func (x URLFetchRequest_RequestMethod) String() string {
	return proto.EnumName(URLFetchRequest_RequestMethod_name, int32(x))
}

func (x *URLFetchRequest_RequestMethod) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(URLFetchRequest_RequestMethod_value, data, "URLFetchRequest_RequestMethod")
	if err != nil {
		return err
	}
	*x = URLFetchRequest_RequestMethod(value)
	return nil
}

func (URLFetchRequest_RequestMethod) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_urlfetch_service_b245a7065f33bced, []int{1, 0}
}

type URLFetchServiceError struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *URLFetchServiceError) Reset()         { *m = URLFetchServiceError{} }
func (m *URLFetchServiceError) String() string { return proto.CompactTextString(m) }
func (*URLFetchServiceError) ProtoMessage()    {}
func (*URLFetchServiceError) Descriptor() ([]byte, []int) {
	return fileDescriptor_urlfetch_service_b245a7065f33bced, []int{0}
}

func (m *URLFetchServiceError) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_URLFetchServiceError.Unmarshal(m, b)
}

func (m *URLFetchServiceError) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_URLFetchServiceError.Marshal(b, m, deterministic)
}

func (dst *URLFetchServiceError) XXX_Merge(src proto.Message) {
	xxx_messageInfo_URLFetchServiceError.Merge(dst, src)
}

func (m *URLFetchServiceError) XXX_Size() int {
	return xxx_messageInfo_URLFetchServiceError.Size(m)
}

func (m *URLFetchServiceError) XXX_DiscardUnknown() {
	xxx_messageInfo_URLFetchServiceError.DiscardUnknown(m)
}

var xxx_messageInfo_URLFetchServiceError proto.InternalMessageInfo

type URLFetchRequest struct {
	Method                        *URLFetchRequest_RequestMethod `protobuf:"varint,1,req,name=Method,enum=appengine.URLFetchRequest_RequestMethod" json:"Method,omitempty"`
	Url                           *string                        `protobuf:"bytes,2,req,name=Url" json:"Url,omitempty"`
	Header                        []*URLFetchRequest_Header      `protobuf:"group,3,rep,name=Header,json=header" json:"header,omitempty"`
	Payload                       []byte                         `protobuf:"bytes,6,opt,name=Payload" json:"Payload,omitempty"`
	FollowRedirects               *bool                          `protobuf:"varint,7,opt,name=FollowRedirects,def=1" json:"FollowRedirects,omitempty"`
	Deadline                      *float64                       `protobuf:"fixed64,8,opt,name=Deadline" json:"Deadline,omitempty"`
	MustValidateServerCertificate *bool                          `protobuf:"varint,9,opt,name=MustValidateServerCertificate,def=1" json:"MustValidateServerCertificate,omitempty"`
	XXX_NoUnkeyedLiteral          struct{}                       `json:"-"`
	XXX_unrecognized              []byte                         `json:"-"`
	XXX_sizecache                 int32                          `json:"-"`
}

func (m *URLFetchRequest) Reset()         { *m = URLFetchRequest{} }
func (m *URLFetchRequest) String() string { return proto.CompactTextString(m) }
func (*URLFetchRequest) ProtoMessage()    {}
func (*URLFetchRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_urlfetch_service_b245a7065f33bced, []int{1}
}

func (m *URLFetchRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_URLFetchRequest.Unmarshal(m, b)
}

func (m *URLFetchRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_URLFetchRequest.Marshal(b, m, deterministic)
}

func (dst *URLFetchRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_URLFetchRequest.Merge(dst, src)
}

func (m *URLFetchRequest) XXX_Size() int {
	return xxx_messageInfo_URLFetchRequest.Size(m)
}

func (m *URLFetchRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_URLFetchRequest.DiscardUnknown(m)
}

var xxx_messageInfo_URLFetchRequest proto.InternalMessageInfo

const (
	Default_URLFetchRequest_FollowRedirects               bool = true
	Default_URLFetchRequest_MustValidateServerCertificate bool = true
)

func (m *URLFetchRequest) GetMethod() URLFetchRequest_RequestMethod {
	if m != nil && m.Method != nil {
		return *m.Method
	}
	return URLFetchRequest_GET
}

func (m *URLFetchRequest) GetUrl() string {
	if m != nil && m.Url != nil {
		return *m.Url
	}
	return ""
}

func (m *URLFetchRequest) GetHeader() []*URLFetchRequest_Header {
	if m != nil {
		return m.Header
	}
	return nil
}

func (m *URLFetchRequest) GetPayload() []byte {
	if m != nil {
		return m.Payload
	}
	return nil
}

func (m *URLFetchRequest) GetFollowRedirects() bool {
	if m != nil && m.FollowRedirects != nil {
		return *m.FollowRedirects
	}
	return Default_URLFetchRequest_FollowRedirects
}

func (m *URLFetchRequest) GetDeadline() float64 {
	if m != nil && m.Deadline != nil {
		return *m.Deadline
	}
	return 0
}

func (m *URLFetchRequest) GetMustValidateServerCertificate() bool {
	if m != nil && m.MustValidateServerCertificate != nil {
		return *m.MustValidateServerCertificate
	}
	return Default_URLFetchRequest_MustValidateServerCertificate
}

type URLFetchRequest_Header struct {
	Key                  *string  `protobuf:"bytes,4,req,name=Key" json:"Key,omitempty"`
	Value                *string  `protobuf:"bytes,5,req,name=Value" json:"Value,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *URLFetchRequest_Header) Reset()         { *m = URLFetchRequest_Header{} }
func (m *URLFetchRequest_Header) String() string { return proto.CompactTextString(m) }
func (*URLFetchRequest_Header) ProtoMessage()    {}
func (*URLFetchRequest_Header) Descriptor() ([]byte, []int) {
	return fileDescriptor_urlfetch_service_b245a7065f33bced, []int{1, 0}
}

func (m *URLFetchRequest_Header) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_URLFetchRequest_Header.Unmarshal(m, b)
}

func (m *URLFetchRequest_Header) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_URLFetchRequest_Header.Marshal(b, m, deterministic)
}

func (dst *URLFetchRequest_Header) XXX_Merge(src proto.Message) {
	xxx_messageInfo_URLFetchRequest_Header.Merge(dst, src)
}

func (m *URLFetchRequest_Header) XXX_Size() int {
	return xxx_messageInfo_URLFetchRequest_Header.Size(m)
}

func (m *URLFetchRequest_Header) XXX_DiscardUnknown() {
	xxx_messageInfo_URLFetchRequest_Header.DiscardUnknown(m)
}

var xxx_messageInfo_URLFetchRequest_Header proto.InternalMessageInfo

func (m *URLFetchRequest_Header) GetKey() string {
	if m != nil && m.Key != nil {
		return *m.Key
	}
	return ""
}

func (m *URLFetchRequest_Header) GetValue() string {
	if m != nil && m.Value != nil {
		return *m.Value
	}
	return ""
}

type URLFetchResponse struct {
	Content               []byte                     `protobuf:"bytes,1,opt,name=Content" json:"Content,omitempty"`
	StatusCode            *int32                     `protobuf:"varint,2,req,name=StatusCode" json:"StatusCode,omitempty"`
	Header                []*URLFetchResponse_Header `protobuf:"group,3,rep,name=Header,json=header" json:"header,omitempty"`
	ContentWasTruncated   *bool                      `protobuf:"varint,6,opt,name=ContentWasTruncated,def=0" json:"ContentWasTruncated,omitempty"`
	ExternalBytesSent     *int64                     `protobuf:"varint,7,opt,name=ExternalBytesSent" json:"ExternalBytesSent,omitempty"`
	ExternalBytesReceived *int64                     `protobuf:"varint,8,opt,name=ExternalBytesReceived" json:"ExternalBytesReceived,omitempty"`
	FinalUrl              *string                    `protobuf:"bytes,9,opt,name=FinalUrl" json:"FinalUrl,omitempty"`
	ApiCpuMilliseconds    *int64                     `protobuf:"varint,10,opt,name=ApiCpuMilliseconds,def=0" json:"ApiCpuMilliseconds,omitempty"`
	ApiBytesSent          *int64                     `protobuf:"varint,11,opt,name=ApiBytesSent,def=0" json:"ApiBytesSent,omitempty"`
	ApiBytesReceived      *int64                     `protobuf:"varint,12,opt,name=ApiBytesReceived,def=0" json:"ApiBytesReceived,omitempty"`
	XXX_NoUnkeyedLiteral  struct{}                   `json:"-"`
	XXX_unrecognized      []byte                     `json:"-"`
	XXX_sizecache         int32                      `json:"-"`
}

func (m *URLFetchResponse) Reset()         { *m = URLFetchResponse{} }
func (m *URLFetchResponse) String() string { return proto.CompactTextString(m) }
func (*URLFetchResponse) ProtoMessage()    {}
func (*URLFetchResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_urlfetch_service_b245a7065f33bced, []int{2}
}

func (m *URLFetchResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_URLFetchResponse.Unmarshal(m, b)
}

func (m *URLFetchResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_URLFetchResponse.Marshal(b, m, deterministic)
}

func (dst *URLFetchResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_URLFetchResponse.Merge(dst, src)
}

func (m *URLFetchResponse) XXX_Size() int {
	return xxx_messageInfo_URLFetchResponse.Size(m)
}

func (m *URLFetchResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_URLFetchResponse.DiscardUnknown(m)
}

var xxx_messageInfo_URLFetchResponse proto.InternalMessageInfo

const (
	Default_URLFetchResponse_ContentWasTruncated bool  = false
	Default_URLFetchResponse_ApiCpuMilliseconds  int64 = 0
	Default_URLFetchResponse_ApiBytesSent        int64 = 0
	Default_URLFetchResponse_ApiBytesReceived    int64 = 0
)

func (m *URLFetchResponse) GetContent() []byte {
	if m != nil {
		return m.Content
	}
	return nil
}

func (m *URLFetchResponse) GetStatusCode() int32 {
	if m != nil && m.StatusCode != nil {
		return *m.StatusCode
	}
	return 0
}

func (m *URLFetchResponse) GetHeader() []*URLFetchResponse_Header {
	if m != nil {
		return m.Header
	}
	return nil
}

func (m *URLFetchResponse) GetContentWasTruncated() bool {
	if m != nil && m.ContentWasTruncated != nil {
		return *m.ContentWasTruncated
	}
	return Default_URLFetchResponse_ContentWasTruncated
}

func (m *URLFetchResponse) GetExternalBytesSent() int64 {
	if m != nil && m.ExternalBytesSent != nil {
		return *m.ExternalBytesSent
	}
	return 0
}

func (m *URLFetchResponse) GetExternalBytesReceived() int64 {
	if m != nil && m.ExternalBytesReceived != nil {
		return *m.ExternalBytesReceived
	}
	return 0
}

func (m *URLFetchResponse) GetFinalUrl() string {
	if m != nil && m.FinalUrl != nil {
		return *m.FinalUrl
	}
	return ""
}

func (m *URLFetchResponse) GetApiCpuMilliseconds() int64 {
	if m != nil && m.ApiCpuMilliseconds != nil {
		return *m.ApiCpuMilliseconds
	}
	return Default_URLFetchResponse_ApiCpuMilliseconds
}

func (m *URLFetchResponse) GetApiBytesSent() int64 {
	if m != nil && m.ApiBytesSent != nil {
		return *m.ApiBytesSent
	}
	return Default_URLFetchResponse_ApiBytesSent
}

func (m *URLFetchResponse) GetApiBytesReceived() int64 {
	if m != nil && m.ApiBytesReceived != nil {
		return *m.ApiBytesReceived
	}
	return Default_URLFetchResponse_ApiBytesReceived
}

type URLFetchResponse_Header struct {
	Key                  *string  `protobuf:"bytes,4,req,name=Key" json:"Key,omitempty"`
	Value                *string  `protobuf:"bytes,5,req,name=Value" json:"Value,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *URLFetchResponse_Header) Reset()         { *m = URLFetchResponse_Header{} }
func (m *URLFetchResponse_Header) String() string { return proto.CompactTextString(m) }
func (*URLFetchResponse_Header) ProtoMessage()    {}
func (*URLFetchResponse_Header) Descriptor() ([]byte, []int) {
	return fileDescriptor_urlfetch_service_b245a7065f33bced, []int{2, 0}
}

func (m *URLFetchResponse_Header) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_URLFetchResponse_Header.Unmarshal(m, b)
}

func (m *URLFetchResponse_Header) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_URLFetchResponse_Header.Marshal(b, m, deterministic)
}

func (dst *URLFetchResponse_Header) XXX_Merge(src proto.Message) {
	xxx_messageInfo_URLFetchResponse_Header.Merge(dst, src)
}

func (m *URLFetchResponse_Header) XXX_Size() int {
	return xxx_messageInfo_URLFetchResponse_Header.Size(m)
}

func (m *URLFetchResponse_Header) XXX_DiscardUnknown() {
	xxx_messageInfo_URLFetchResponse_Header.DiscardUnknown(m)
}

var xxx_messageInfo_URLFetchResponse_Header proto.InternalMessageInfo

func (m *URLFetchResponse_Header) GetKey() string {
	if m != nil && m.Key != nil {
		return *m.Key
	}
	return ""
}

func (m *URLFetchResponse_Header) GetValue() string {
	if m != nil && m.Value != nil {
		return *m.Value
	}
	return ""
}

func init() {
	proto.RegisterType((*URLFetchServiceError)(nil), "appengine.URLFetchServiceError")
	proto.RegisterType((*URLFetchRequest)(nil), "appengine.URLFetchRequest")
	proto.RegisterType((*URLFetchRequest_Header)(nil), "appengine.URLFetchRequest.Header")
	proto.RegisterType((*URLFetchResponse)(nil), "appengine.URLFetchResponse")
	proto.RegisterType((*URLFetchResponse_Header)(nil), "appengine.URLFetchResponse.Header")
}

func init() {
	proto.RegisterFile("google.golang.org/appengine/internal/urlfetch/urlfetch_service.proto", fileDescriptor_urlfetch_service_b245a7065f33bced)
}

var fileDescriptor_urlfetch_service_b245a7065f33bced = []byte{
	// 770 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x94, 0x54, 0xdd, 0x6e, 0xe3, 0x54,
	0x10, 0xc6, 0x76, 0x7e, 0xa7, 0x5d, 0x7a, 0x76, 0xb6, 0x45, 0x66, 0xb5, 0xa0, 0x10, 0x09, 0x29,
	0x17, 0x90, 0x2e, 0x2b, 0x24, 0x44, 0xaf, 0x70, 0xed, 0x93, 0xad, 0xa9, 0x63, 0x47, 0xc7, 0x4e,
	0x61, 0xb9, 0xb1, 0xac, 0x78, 0x9a, 0x5a, 0xb2, 0xec, 0x60, 0x9f, 0x2c, 0xf4, 0x35, 0x78, 0x0d,
	0xde, 0x87, 0xa7, 0xe1, 0x02, 0x9d, 0xc4, 0xc9, 0x6e, 0xbb, 0xd1, 0x4a, 0x5c, 0x65, 0xe6, 0x9b,
	0xef, 0xcc, 0x99, 0x7c, 0xdf, 0xf8, 0x80, 0xb3, 0x2c, 0xcb, 0x65, 0x4e, 0xe3, 0x65, 0x99, 0x27,
	0xc5, 0x72, 0x5c, 0x56, 0xcb, 0xf3, 0x64, 0xb5, 0xa2, 0x62, 0x99, 0x15, 0x74, 0x9e, 0x15, 0x92,
	0xaa, 0x22, 0xc9, 0xcf, 0xd7, 0x55, 0x7e, 0x4b, 0x72, 0x71, 0xb7, 0x0f, 0xe2, 0x9a, 0xaa, 0xb7,
	0xd9, 0x82, 0xc6, 0xab, 0xaa, 0x94, 0x25, 0xf6, 0xf7, 0x67, 0x86, 0x7f, 0xeb, 0x70, 0x3a, 0x17,
	0xde, 0x44, 0xb1, 0xc2, 0x2d, 0x89, 0x57, 0x55, 0x59, 0x0d, 0xff, 0xd2, 0xa1, 0xbf, 0x89, 0xec,
	0x32, 0x25, 0xec, 0x80, 0x1e, 0x5c, 0xb3, 0x4f, 0xf0, 0x04, 0x8e, 0x5c, 0xff, 0xc6, 0xf2, 0x5c,
	0x27, 0x9e, 0x0b, 0x8f, 0x69, 0x0a, 0x98, 0xf0, 0xc8, 0xbe, 0x8a, 0xb9, 0x10, 0x81, 0x60, 0x3a,
	0x9e, 0xc1, 0xd3, 0xb9, 0x1f, 0xce, 0xb8, 0xed, 0x4e, 0x5c, 0xee, 0x34, 0xb0, 0x81, 0x9f, 0x01,
	0x0a, 0x1e, 0xce, 0x02, 0x3f, 0xe4, 0x71, 0x14, 0x04, 0xb1, 0x67, 0x89, 0xd7, 0x9c, 0xb5, 0x14,
	0xdd, 0xe1, 0x96, 0xe3, 0xb9, 0x3e, 0x8f, 0xf9, 0xaf, 0x36, 0xe7, 0x0e, 0x77, 0x58, 0x1b, 0x3f,
	0x87, 0xb3, 0x30, 0xf4, 0x62, 0x9b, 0x8b, 0xc8, 0x9d, 0xb8, 0xb6, 0x15, 0xf1, 0xa6, 0x53, 0x07,
	0x9f, 0x40, 0xdf, 0xf1, 0xc3, 0x26, 0xed, 0x22, 0x40, 0xc7, 0xf6, 0x82, 0x90, 0x3b, 0xac, 0x87,
	0x2f, 0xc0, 0x74, 0xfd, 0x88, 0x0b, 0xdf, 0xf2, 0xe2, 0x48, 0x58, 0x7e, 0xe8, 0x72, 0x3f, 0x6a,
	0x98, 0x7d, 0x35, 0x82, 0xba, 0x79, 0x6a, 0xf9, 0x6f, 0x62, 0xc1, 0x1d, 0x57, 0x70, 0x3b, 0x0a,
	0x19, 0xe0, 0x33, 0x38, 0x99, 0x5a, 0xde, 0x24, 0x10, 0x53, 0xee, 0xc4, 0x82, 0xcf, 0xbc, 0x37,
	0xec, 0x08, 0x4f, 0x81, 0xd9, 0x81, 0xef, 0x73, 0x3b, 0x72, 0x03, 0xbf, 0x69, 0x71, 0x3c, 0xfc,
	0xc7, 0x80, 0x93, 0x9d, 0x5a, 0x82, 0x7e, 0x5f, 0x53, 0x2d, 0xf1, 0x27, 0xe8, 0x4c, 0x49, 0xde,
	0x95, 0xa9, 0xa9, 0x0d, 0xf4, 0xd1, 0xa7, 0xaf, 0x46, 0xe3, 0xbd, 0xba, 0xe3, 0x47, 0xdc, 0x71,
	0xf3, 0xbb, 0xe5, 0x8b, 0xe6, 0x1c, 0x32, 0x30, 0xe6, 0x55, 0x6e, 0xea, 0x03, 0x7d, 0xd4, 0x17,
	0x2a, 0xc4, 0x1f, 0xa1, 0x73, 0x47, 0x49, 0x4a, 0x95, 0x69, 0x0c, 0x8c, 0x11, 0xbc, 0xfa, 0xea,
	0x23, 0x3d, 0xaf, 0x36, 0x44, 0xd1, 0x1c, 0xc0, 0x17, 0xd0, 0x9d, 0x25, 0xf7, 0x79, 0x99, 0xa4,
	0x66, 0x67, 0xa0, 0x8d, 0x8e, 0x2f, 0xf5, 0x9e, 0x26, 0x76, 0x10, 0x8e, 0xe1, 0x64, 0x52, 0xe6,
	0x79, 0xf9, 0x87, 0xa0, 0x34, 0xab, 0x68, 0x21, 0x6b, 0xb3, 0x3b, 0xd0, 0x46, 0xbd, 0x8b, 0x96,
	0xac, 0xd6, 0x24, 0x1e, 0x17, 0xf1, 0x39, 0xf4, 0x1c, 0x4a, 0xd2, 0x3c, 0x2b, 0xc8, 0xec, 0x0d,
	0xb4, 0x91, 0x26, 0xf6, 0x39, 0xfe, 0x0c, 0x5f, 0x4c, 0xd7, 0xb5, 0xbc, 0x49, 0xf2, 0x2c, 0x4d,
	0x24, 0xa9, 0xed, 0xa1, 0xca, 0xa6, 0x4a, 0x66, 0xb7, 0xd9, 0x22, 0x91, 0x64, 0xf6, 0xdf, 0xeb,
	0xfc, 0x71, 0xea, 0xf3, 0x97, 0xd0, 0xd9, 0xfe, 0x0f, 0x25, 0xc6, 0x35, 0xdd, 0x9b, 0xad, 0xad,
	0x18, 0xd7, 0x74, 0x8f, 0xa7, 0xd0, 0xbe, 0x49, 0xf2, 0x35, 0x99, 0xed, 0x0d, 0xb6, 0x4d, 0x86,
	0x1e, 0x3c, 0x79, 0xa0, 0x26, 0x76, 0xc1, 0x78, 0xcd, 0x23, 0xa6, 0x61, 0x0f, 0x5a, 0xb3, 0x20,
	0x8c, 0x98, 0xae, 0xa2, 0x2b, 0x6e, 0x39, 0xcc, 0x50, 0xc5, 0xd9, 0x3c, 0x62, 0x2d, 0xb5, 0x2e,
	0x0e, 0xf7, 0x78, 0xc4, 0x59, 0x1b, 0xfb, 0xd0, 0x9e, 0x59, 0x91, 0x7d, 0xc5, 0x3a, 0xc3, 0x7f,
	0x0d, 0x60, 0xef, 0x84, 0xad, 0x57, 0x65, 0x51, 0x13, 0x9a, 0xd0, 0xb5, 0xcb, 0x42, 0x52, 0x21,
	0x4d, 0x4d, 0x49, 0x29, 0x76, 0x29, 0x7e, 0x09, 0x10, 0xca, 0x44, 0xae, 0x6b, 0xf5, 0x71, 0x6c,
	0x8c, 0x6b, 0x8b, 0xf7, 0x10, 0xbc, 0x78, 0xe4, 0xdf, 0xf0, 0xa0, 0x7f, 0xdb, 0x6b, 0x1e, 0x1b,
	0xf8, 0x03, 0x3c, 0x6b, 0xae, 0xf9, 0x25, 0xa9, 0xa3, 0x6a, 0x5d, 0x28, 0x81, 0xb6, 0x66, 0xf6,
	0x2e, 0xda, 0xb7, 0x49, 0x5e, 0x93, 0x38, 0xc4, 0xc0, 0x6f, 0xe0, 0x29, 0xff, 0x73, 0xfb, 0x02,
	0x5c, 0xde, 0x4b, 0xaa, 0x43, 0x35, 0xb8, 0x72, 0xd7, 0x10, 0x1f, 0x16, 0xf0, 0x7b, 0x38, 0x7b,
	0x00, 0x0a, 0x5a, 0x50, 0xf6, 0x96, 0xd2, 0x8d, 0xcd, 0x86, 0x38, 0x5c, 0x54, 0xfb, 0x30, 0xc9,
	0x8a, 0x24, 0x57, 0xfb, 0xaa, 0xec, 0xed, 0x8b, 0x7d, 0x8e, 0xdf, 0x01, 0x5a, 0xab, 0xcc, 0x5e,
	0xad, 0xa7, 0x59, 0x9e, 0x67, 0x35, 0x2d, 0xca, 0x22, 0xad, 0x4d, 0x50, 0xed, 0x2e, 0xb4, 0x97,
	0xe2, 0x40, 0x11, 0xbf, 0x86, 0x63, 0x6b, 0x95, 0xbd, 0x9b, 0xf6, 0x68, 0x47, 0x7e, 0x00, 0xe3,
	0xb7, 0xc0, 0x76, 0xf9, 0x7e, 0xcc, 0xe3, 0x1d, 0xf5, 0x83, 0xd2, 0xff, 0x5f, 0xa6, 0x4b, 0xf8,
	0xad, 0xb7, 0x7b, 0x2a, 0xff, 0x0b, 0x00, 0x00, 0xff, 0xff, 0x1d, 0x9f, 0x6d, 0x24, 0x63, 0x05,
	0x00, 0x00,
}
