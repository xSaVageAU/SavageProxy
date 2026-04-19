package intercept

// ProxyCommand defines a command owned by the proxy.
type ProxyCommand struct {
	Name        string
	Subcommands []string // Optional subcommand literals (e.g., "reload", "status")
}

// ProxyCommands is the registry of all proxy-owned commands.
var ProxyCommands = []ProxyCommand{
	{Name: "savage"},
}
