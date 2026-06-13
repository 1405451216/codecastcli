# ----------------------------------------------------------------------
# Fish completion for codecast CLI
# ----------------------------------------------------------------------
# Installation:
#   Copy to Fish completions directory:
#     cp codecast.fish ~/.config/fish/completions/codecast.fish
#   Or system-wide:
#     cp codecast.fish /usr/share/fish/vendor_completions.d/codecast.fish
# ----------------------------------------------------------------------

# Disable file completions by default
complete -c codecast -f

# ---- Top-level subcommands ----
complete -c codecast -n '__fish_use_subcommand' -a exec -d 'Headless mode - execute a single prompt'
complete -c codecast -n '__fish_use_subcommand' -a init -d 'Initialize project configuration'
complete -c codecast -n '__fish_use_subcommand' -a config -d 'Configuration management'
complete -c codecast -n '__fish_use_subcommand' -a plugin -d 'Manage community plugins'
complete -c codecast -n '__fish_use_subcommand' -a pool -d 'Manage Agent Pool'
complete -c codecast -n '__fish_use_subcommand' -a sandbox -d 'Sandbox isolation management'
complete -c codecast -n '__fish_use_subcommand' -a mcp -d 'MCP Server management'
complete -c codecast -n '__fish_use_subcommand' -a cost -d 'Cost tracking management'

# ---- Global flags ----
complete -c codecast -n '__fish_use_subcommand' -l model -s m -d 'AI model to use' -r
complete -c codecast -n '__fish_use_subcommand' -l provider -s p -d 'LLM provider' -r -a 'openai anthropic gemini ollama deepseek qwen glm'
complete -c codecast -n '__fish_use_subcommand' -l api-key -s k -d 'API key' -r
complete -c codecast -n '__fish_use_subcommand' -l base-url -s u -d 'API base URL' -r
complete -c codecast -n '__fish_use_subcommand' -l permission-mode -d 'Permission approval mode' -r -a 'suggest auto-edit full-auto'
complete -c codecast -n '__fish_use_subcommand' -l scope -d 'File access scope' -r -F
complete -c codecast -n '__fish_use_subcommand' -l safe -d 'Safe mode - disable Shell and Web tools'
complete -c codecast -n '__fish_use_subcommand' -l continue -s c -d 'Continue the most recent session'
complete -c codecast -n '__fish_use_subcommand' -l resume -s r -d 'Resume a specific session ID' -r
complete -c codecast -n '__fish_use_subcommand' -l log-level -d 'Log level' -r -a 'debug info warn error'
complete -c codecast -n '__fish_use_subcommand' -l log-format -d 'Log format' -r -a 'text json'
complete -c codecast -n '__fish_use_subcommand' -l config -d 'Config file path' -r -F

# Global flags also available with subcommands
complete -c codecast -n '__fish_seen_subcommand_from exec init config plugin pool sandbox mcp cost' -l model -s m -d 'AI model to use' -r
complete -c codecast -n '__fish_seen_subcommand_from exec init config plugin pool sandbox mcp cost' -l provider -s p -d 'LLM provider' -r -a 'openai anthropic gemini ollama deepseek qwen glm'
complete -c codecast -n '__fish_seen_subcommand_from exec init config plugin pool sandbox mcp cost' -l api-key -s k -d 'API key' -r
complete -c codecast -n '__fish_seen_subcommand_from exec init config plugin pool sandbox mcp cost' -l base-url -s u -d 'API base URL' -r
complete -c codecast -n '__fish_seen_subcommand_from exec init config plugin pool sandbox mcp cost' -l permission-mode -d 'Permission approval mode' -r -a 'suggest auto-edit full-auto'
complete -c codecast -n '__fish_seen_subcommand_from exec init config plugin pool sandbox mcp cost' -l scope -d 'File access scope' -r -F
complete -c codecast -n '__fish_seen_subcommand_from exec init config plugin pool sandbox mcp cost' -l safe -d 'Safe mode'
complete -c codecast -n '__fish_seen_subcommand_from exec init config plugin pool sandbox mcp cost' -l continue -s c -d 'Continue the most recent session'
complete -c codecast -n '__fish_seen_subcommand_from exec init config plugin pool sandbox mcp cost' -l resume -s r -d 'Resume a specific session ID' -r
complete -c codecast -n '__fish_seen_subcommand_from exec init config plugin pool sandbox mcp cost' -l log-level -d 'Log level' -r -a 'debug info warn error'
complete -c codecast -n '__fish_seen_subcommand_from exec init config plugin pool sandbox mcp cost' -l log-format -d 'Log format' -r -a 'text json'
complete -c codecast -n '__fish_seen_subcommand_from exec init config plugin pool sandbox mcp cost' -l config -d 'Config file path' -r -F

# ---- exec subcommand ----
complete -c codecast -n '__fish_seen_subcommand_from exec' -l format -s f -d 'Output format' -r -a 'text json stream-json'
complete -c codecast -n '__fish_seen_subcommand_from exec' -l model -s m -d 'Override model' -r
complete -c codecast -n '__fish_seen_subcommand_from exec' -l timeout -s t -d 'Timeout in seconds' -r

# ---- config subcommand ----
complete -c codecast -n '__fish_seen_subcommand_from config' -a 'set' -d 'Set a configuration value'
complete -c codecast -n '__fish_seen_subcommand_from config' -a 'get' -d 'Get a configuration value'
complete -c codecast -n '__fish_seen_subcommand_from config' -a 'list' -d 'List all configuration'
complete -c codecast -n '__fish_seen_subcommand_from config' -a 'init' -d 'Initialize config file'
complete -c codecast -n '__fish_seen_subcommand_from config' -a 'providers' -d 'List supported LLM providers'

# config set/get key completion
complete -c codecast -n '__fish_seen_subcommand_from config; and __fish_seen_subcommand_from set get' -a 'api_key' -d 'API key'
complete -c codecast -n '__fish_seen_subcommand_from config; and __fish_seen_subcommand_from set get' -a 'model' -d 'Default model'
complete -c codecast -n '__fish_seen_subcommand_from config; and __fish_seen_subcommand_from set get' -a 'provider' -d 'Default provider'
complete -c codecast -n '__fish_seen_subcommand_from config; and __fish_seen_subcommand_from set get' -a 'base_url' -d 'API base URL'

# ---- plugin subcommand ----
complete -c codecast -n '__fish_seen_subcommand_from plugin' -a 'list' -d 'List installed plugins'
complete -c codecast -n '__fish_seen_subcommand_from plugin' -a 'available' -d 'List available plugins'
complete -c codecast -n '__fish_seen_subcommand_from plugin' -a 'install' -d 'Install a plugin'
complete -c codecast -n '__fish_seen_subcommand_from plugin' -a 'unload' -d 'Unload a plugin'
complete -c codecast -n '__fish_seen_subcommand_from plugin' -l dir -d 'Plugin install directory' -r -F

# ---- pool subcommand ----
complete -c codecast -n '__fish_seen_subcommand_from pool' -a 'status' -d 'View Pool status'
complete -c codecast -n '__fish_seen_subcommand_from pool' -a 'run' -d 'Submit parallel tasks'
complete -c codecast -n '__fish_seen_subcommand_from pool' -l concurrency -d 'Maximum concurrency' -r

# ---- sandbox subcommand ----
complete -c codecast -n '__fish_seen_subcommand_from sandbox' -a 'status' -d 'View sandbox status'
complete -c codecast -n '__fish_seen_subcommand_from sandbox' -a 'build' -d 'Build sandbox image'

# ---- mcp subcommand ----
complete -c codecast -n '__fish_seen_subcommand_from mcp' -a 'list' -d 'List registered MCP servers'
complete -c codecast -n '__fish_seen_subcommand_from mcp' -a 'add' -d 'Register an MCP server'
complete -c codecast -n '__fish_seen_subcommand_from mcp' -a 'remove' -d 'Remove an MCP server'
complete -c codecast -n '__fish_seen_subcommand_from mcp' -a 'test' -d 'Test MCP server connection'
complete -c codecast -n '__fish_seen_subcommand_from mcp' -a 'templates' -d 'List available MCP server templates'
complete -c codecast -n '__fish_seen_subcommand_from mcp' -a 'categories' -d 'List MCP server categories'

# mcp add flags
complete -c codecast -n '__fish_seen_subcommand_from mcp; and __fish_seen_subcommand_from add' -l command -s c -d 'Start command' -r
complete -c codecast -n '__fish_seen_subcommand_from mcp; and __fish_seen_subcommand_from add' -l args -s a -d 'Command arguments' -r
complete -c codecast -n '__fish_seen_subcommand_from mcp; and __fish_seen_subcommand_from add' -l url -s u -d 'Server URL' -r
complete -c codecast -n '__fish_seen_subcommand_from mcp; and __fish_seen_subcommand_from add' -l auto-start -d 'Auto start' -r -a 'true false'

# ---- cost subcommand ----
complete -c codecast -n '__fish_seen_subcommand_from cost' -a 'summary' -d 'View cost summary'
complete -c codecast -n '__fish_seen_subcommand_from cost' -a 'daily' -d 'View daily cost statistics'
complete -c codecast -n '__fish_seen_subcommand_from cost' -a 'list' -d 'View recent call records'
complete -c codecast -n '__fish_seen_subcommand_from cost' -a 'clear' -d 'Clear all cost records'

# cost subcommand flags
complete -c codecast -n '__fish_seen_subcommand_from cost; and __fish_seen_subcommand_from summary daily list' -l json -d 'Output in JSON format'
