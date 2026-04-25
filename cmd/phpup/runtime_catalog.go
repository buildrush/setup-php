package main

import "github.com/buildrush/setup-php/internal/catalog"

// runtimeExtensionSpecs returns the extension specs used by `phpup install`
// at runtime. These MUST stay in sync with catalog/extensions/*.yaml — see
// TestRuntimeExtensionSpecsMatchYAML in runtime_catalog_test.go for the
// drift guard.
func runtimeExtensionSpecs() map[string]*catalog.ExtensionSpec {
	return map[string]*catalog.ExtensionSpec{
		"redis":     {Name: "redis", Kind: catalog.ExtensionKindPECL, Versions: []string{"6.3.0"}},
		"xdebug":    {Name: "xdebug", Kind: catalog.ExtensionKindPECL, Versions: []string{"3.5.1"}, Ini: []string{"zend_extension=xdebug"}},
		"pcov":      {Name: "pcov", Kind: catalog.ExtensionKindPECL, Versions: []string{"1.0.12"}, Ini: []string{"extension=pcov"}},
		"apcu":      {Name: "apcu", Kind: catalog.ExtensionKindPECL, Versions: []string{"5.1.28"}},
		"igbinary":  {Name: "igbinary", Kind: catalog.ExtensionKindPECL, Versions: []string{"3.2.16"}},
		"msgpack":   {Name: "msgpack", Kind: catalog.ExtensionKindPECL, Versions: []string{"3.0.0"}},
		"uuid":      {Name: "uuid", Kind: catalog.ExtensionKindPECL, Versions: []string{"1.3.0"}, RuntimeDeps: map[string][]string{"linux": {"libuuid1"}}},
		"ssh2":      {Name: "ssh2", Kind: catalog.ExtensionKindPECL, Versions: []string{"1.5.0"}, RuntimeDeps: map[string][]string{"linux": {"libssh2-1"}}},
		"yaml":      {Name: "yaml", Kind: catalog.ExtensionKindPECL, Versions: []string{"2.3.0"}, RuntimeDeps: map[string][]string{"linux": {"libyaml-0-2"}}},
		"memcached": {Name: "memcached", Kind: catalog.ExtensionKindPECL, Versions: []string{"3.4.0"}, RuntimeDeps: map[string][]string{"linux": {"libmemcached11", "libsasl2-2"}}},
		"amqp":      {Name: "amqp", Kind: catalog.ExtensionKindPECL, Versions: []string{"2.2.0"}, RuntimeDeps: map[string][]string{"linux": {"librabbitmq4"}}},
		"event":     {Name: "event", Kind: catalog.ExtensionKindPECL, Versions: []string{"3.1.4"}, RuntimeDeps: map[string][]string{"linux": {"libevent-2.1-7", "libevent-extra-2.1-7", "libevent-openssl-2.1-7"}}},
		"rdkafka":   {Name: "rdkafka", Kind: catalog.ExtensionKindPECL, Versions: []string{"6.0.5"}, RuntimeDeps: map[string][]string{"linux": {"librdkafka1"}}},
		"protobuf":  {Name: "protobuf", Kind: catalog.ExtensionKindPECL, Versions: []string{"5.34.1"}},
		"imagick":   {Name: "imagick", Kind: catalog.ExtensionKindPECL, Versions: []string{"3.8.1"}, RuntimeDeps: map[string][]string{"linux": {"libfontconfig1", "libx11-6", "libxext6", "liblcms2-2", "liblqr-1-0", "libfftw3-double3", "libbz2-1.0"}}},
		"mongodb":   {Name: "mongodb", Kind: catalog.ExtensionKindPECL, Versions: []string{"2.2.1"}, RuntimeDeps: map[string][]string{"linux": {"libssl3", "libsasl2-2"}}},
		"swoole":    {Name: "swoole", Kind: catalog.ExtensionKindPECL, Versions: []string{"6.2.0"}, RuntimeDeps: map[string][]string{"linux": {"libssl3", "libcurl4"}}},
		"grpc":      {Name: "grpc", Kind: catalog.ExtensionKindPECL, Versions: []string{"1.80.0"}, RuntimeDeps: map[string][]string{"linux": {"libssl3"}}},
	}
}
