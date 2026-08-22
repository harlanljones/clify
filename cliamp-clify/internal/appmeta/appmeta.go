package appmeta

var (
	clientName = "cliamp"
	deviceName = "cliamp"
	version    = "dev"
)

func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

func ClientName() string { return clientName }

func DeviceName() string { return deviceName }

func Version() string { return version }
