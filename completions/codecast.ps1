# ----------------------------------------------------------------------
# PowerShell completion for codecast CLI
# ----------------------------------------------------------------------
# Installation:
#   Add to your PowerShell profile:
#     . /path/to/codecast.ps1
#   Or add to $PROFILE:
#     Add-Content $PROFILE '. /path/to/codecast.ps1'
# ----------------------------------------------------------------------

using namespace System.Management.Automation
using namespace System.Management.Automation.Language

Register-ArgumentCompleter -Native -CommandName codecast -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    # Parse the command line
    $tokens = $commandAst.CommandElements.TokenText
    $subcommand = $null
    $subSubcommand = $null

    # Find subcommand (first non-flag token after 'codecast')
    $tokenList = @($commandAst.CommandElements | ForEach-Object { $_.ToString() })
    for ($i = 1; $i -lt $tokenList.Count; $i++) {
        if ($tokenList[$i] -notmatch '^-') {
            $subcommand = $tokenList[$i]
            # Look for sub-subcommand
            for ($j = $i + 1; $j -lt $tokenList.Count; $j++) {
                if ($tokenList[$j] -notmatch '^-') {
                    $subSubcommand = $tokenList[$j]
                    break
                }
            }
            break
        }
    }

    $completions = @()

    # ---- No subcommand: complete subcommands and global flags ----
    if (-not $subcommand) {
        $subcommands = @(
            [CompletionResult]::new('exec', 'exec', [CompletionResultType]::ParameterValue, 'Headless mode - execute a single prompt')
            [CompletionResult]::new('init', 'init', [CompletionResultType]::ParameterValue, 'Initialize project configuration')
            [CompletionResult]::new('config', 'config', [CompletionResultType]::ParameterValue, 'Configuration management')
            [CompletionResult]::new('plugin', 'plugin', [CompletionResultType]::ParameterValue, 'Manage community plugins')
            [CompletionResult]::new('pool', 'pool', [CompletionResultType]::ParameterValue, 'Manage Agent Pool')
            [CompletionResult]::new('sandbox', 'sandbox', [CompletionResultType]::ParameterValue, 'Sandbox isolation management')
            [CompletionResult]::new('mcp', 'mcp', [CompletionResultType]::ParameterValue, 'MCP Server management')
            [CompletionResult]::new('cost', 'cost', [CompletionResultType]::ParameterValue, 'Cost tracking management')
        )
        $globalFlags = @(
            [CompletionResult]::new('--model', '--model', [CompletionResultType]::ParameterName, 'AI model to use')
            [CompletionResult]::new('-m', '-m', [CompletionResultType]::ParameterName, 'AI model to use')
            [CompletionResult]::new('--provider', '--provider', [CompletionResultType]::ParameterName, 'LLM provider')
            [CompletionResult]::new('-p', '-p', [CompletionResultType]::ParameterName, 'LLM provider')
            [CompletionResult]::new('--api-key', '--api-key', [CompletionResultType]::ParameterName, 'API key')
            [CompletionResult]::new('-k', '-k', [CompletionResultType]::ParameterName, 'API key')
            [CompletionResult]::new('--base-url', '--base-url', [CompletionResultType]::ParameterName, 'API base URL')
            [CompletionResult]::new('-u', '-u', [CompletionResultType]::ParameterName, 'API base URL')
            [CompletionResult]::new('--permission-mode', '--permission-mode', [CompletionResultType]::ParameterName, 'Permission approval mode')
            [CompletionResult]::new('--scope', '--scope', [CompletionResultType]::ParameterName, 'File access scope')
            [CompletionResult]::new('--safe', '--safe', [CompletionResultType]::ParameterName, 'Safe mode')
            [CompletionResult]::new('--continue', '--continue', [CompletionResultType]::ParameterName, 'Continue the most recent session')
            [CompletionResult]::new('-c', '-c', [CompletionResultType]::ParameterName, 'Continue the most recent session')
            [CompletionResult]::new('--resume', '--resume', [CompletionResultType]::ParameterName, 'Resume a specific session ID')
            [CompletionResult]::new('-r', '-r', [CompletionResultType]::ParameterName, 'Resume a specific session ID')
            [CompletionResult]::new('--log-level', '--log-level', [CompletionResultType]::ParameterName, 'Log level')
            [CompletionResult]::new('--log-format', '--log-format', [CompletionResultType]::ParameterName, 'Log format')
            [CompletionResult]::new('--config', '--config', [CompletionResultType]::ParameterName, 'Config file path')
        )
        $completions += $subcommands + $globalFlags
    }
    else {
        # ---- Subcommand-specific completions ----
        switch ($subcommand) {
            'exec' {
                $completions += @(
                    [CompletionResult]::new('--format', '--format', [CompletionResultType]::ParameterName, 'Output format')
                    [CompletionResult]::new('-f', '-f', [CompletionResultType]::ParameterName, 'Output format')
                    [CompletionResult]::new('--model', '--model', [CompletionResultType]::ParameterName, 'Override model')
                    [CompletionResult]::new('-m', '-m', [CompletionResultType]::ParameterName, 'Override model')
                    [CompletionResult]::new('--timeout', '--timeout', [CompletionResultType]::ParameterName, 'Timeout in seconds')
                    [CompletionResult]::new('-t', '-t', [CompletionResultType]::ParameterName, 'Timeout in seconds')
                )
            }
            'init' {
                # No additional completions
            }
            'config' {
                if (-not $subSubcommand) {
                    $completions += @(
                        [CompletionResult]::new('set', 'set', [CompletionResultType]::ParameterValue, 'Set a configuration value')
                        [CompletionResult]::new('get', 'get', [CompletionResultType]::ParameterValue, 'Get a configuration value')
                        [CompletionResult]::new('list', 'list', [CompletionResultType]::ParameterValue, 'List all configuration')
                        [CompletionResult]::new('init', 'init', [CompletionResultType]::ParameterValue, 'Initialize config file')
                        [CompletionResult]::new('providers', 'providers', [CompletionResultType]::ParameterValue, 'List supported LLM providers')
                    )
                }
                elseif ($subSubcommand -in @('set', 'get')) {
                    # Complete config keys
                    $configKeys = @('api_key', 'model', 'provider', 'base_url')
                    foreach ($key in $configKeys) {
                        if ($key -like "$wordToComplete*") {
                            $completions += [CompletionResult]::new($key, $key, [CompletionResultType]::ParameterValue, "Config key: $key")
                        }
                    }
                }
            }
            'plugin' {
                if (-not $subSubcommand) {
                    $completions += @(
                        [CompletionResult]::new('list', 'list', [CompletionResultType]::ParameterValue, 'List installed plugins')
                        [CompletionResult]::new('available', 'available', [CompletionResultType]::ParameterValue, 'List available plugins')
                        [CompletionResult]::new('install', 'install', [CompletionResultType]::ParameterValue, 'Install a plugin')
                        [CompletionResult]::new('unload', 'unload', [CompletionResultType]::ParameterValue, 'Unload a plugin')
                    )
                }
                $completions += [CompletionResult]::new('--dir', '--dir', [CompletionResultType]::ParameterName, 'Plugin install directory')
            }
            'pool' {
                if (-not $subSubcommand) {
                    $completions += @(
                        [CompletionResult]::new('status', 'status', [CompletionResultType]::ParameterValue, 'View Pool status')
                        [CompletionResult]::new('run', 'run', [CompletionResultType]::ParameterValue, 'Submit parallel tasks')
                    )
                }
                $completions += [CompletionResult]::new('--concurrency', '--concurrency', [CompletionResultType]::ParameterName, 'Maximum concurrency')
            }
            'sandbox' {
                if (-not $subSubcommand) {
                    $completions += @(
                        [CompletionResult]::new('status', 'status', [CompletionResultType]::ParameterValue, 'View sandbox status')
                        [CompletionResult]::new('build', 'build', [CompletionResultType]::ParameterValue, 'Build sandbox image')
                    )
                }
            }
            'mcp' {
                if (-not $subSubcommand) {
                    $completions += @(
                        [CompletionResult]::new('list', 'list', [CompletionResultType]::ParameterValue, 'List registered MCP servers')
                        [CompletionResult]::new('add', 'add', [CompletionResultType]::ParameterValue, 'Register an MCP server')
                        [CompletionResult]::new('remove', 'remove', [CompletionResultType]::ParameterValue, 'Remove an MCP server')
                        [CompletionResult]::new('test', 'test', [CompletionResultType]::ParameterValue, 'Test MCP server connection')
                        [CompletionResult]::new('templates', 'templates', [CompletionResultType]::ParameterValue, 'List available MCP server templates')
                        [CompletionResult]::new('categories', 'categories', [CompletionResultType]::ParameterValue, 'List MCP server categories')
                    )
                }
                elseif ($subSubcommand -eq 'add') {
                    $completions += @(
                        [CompletionResult]::new('--command', '--command', [CompletionResultType]::ParameterName, 'Start command')
                        [CompletionResult]::new('-c', '-c', [CompletionResultType]::ParameterName, 'Start command')
                        [CompletionResult]::new('--args', '--args', [CompletionResultType]::ParameterName, 'Command arguments')
                        [CompletionResult]::new('-a', '-a', [CompletionResultType]::ParameterName, 'Command arguments')
                        [CompletionResult]::new('--url', '--url', [CompletionResultType]::ParameterName, 'Server URL')
                        [CompletionResult]::new('-u', '-u', [CompletionResultType]::ParameterName, 'Server URL')
                        [CompletionResult]::new('--auto-start', '--auto-start', [CompletionResultType]::ParameterName, 'Auto start')
                    )
                }
            }
            'cost' {
                if (-not $subSubcommand) {
                    $completions += @(
                        [CompletionResult]::new('summary', 'summary', [CompletionResultType]::ParameterValue, 'View cost summary')
                        [CompletionResult]::new('daily', 'daily', [CompletionResultType]::ParameterValue, 'View daily cost statistics')
                        [CompletionResult]::new('list', 'list', [CompletionResultType]::ParameterValue, 'View recent call records')
                        [CompletionResult]::new('clear', 'clear', [CompletionResultType]::ParameterValue, 'Clear all cost records')
                    )
                }
                elseif ($subSubcommand -in @('summary', 'daily', 'list')) {
                    $completions += [CompletionResult]::new('--json', '--json', [CompletionResultType]::ParameterName, 'Output in JSON format')
                }
            }
        }

        # Add global flags for any subcommand context
        $completions += @(
            [CompletionResult]::new('--model', '--model', [CompletionResultType]::ParameterName, 'AI model to use')
            [CompletionResult]::new('-m', '-m', [CompletionResultType]::ParameterName, 'AI model to use')
            [CompletionResult]::new('--provider', '--provider', [CompletionResultType]::ParameterName, 'LLM provider')
            [CompletionResult]::new('-p', '-p', [CompletionResultType]::ParameterName, 'LLM provider')
            [CompletionResult]::new('--api-key', '--api-key', [CompletionResultType]::ParameterName, 'API key')
            [CompletionResult]::new('-k', '-k', [CompletionResultType]::ParameterName, 'API key')
            [CompletionResult]::new('--base-url', '--base-url', [CompletionResultType]::ParameterName, 'API base URL')
            [CompletionResult]::new('-u', '-u', [CompletionResultType]::ParameterName, 'API base URL')
            [CompletionResult]::new('--permission-mode', '--permission-mode', [CompletionResultType]::ParameterName, 'Permission approval mode')
            [CompletionResult]::new('--scope', '--scope', [CompletionResultType]::ParameterName, 'File access scope')
            [CompletionResult]::new('--safe', '--safe', [CompletionResultType]::ParameterName, 'Safe mode')
            [CompletionResult]::new('--continue', '--continue', [CompletionResultType]::ParameterName, 'Continue the most recent session')
            [CompletionResult]::new('-c', '-c', [CompletionResultType]::ParameterName, 'Continue the most recent session')
            [CompletionResult]::new('--resume', '--resume', [CompletionResultType]::ParameterName, 'Resume a specific session ID')
            [CompletionResult]::new('-r', '-r', [CompletionResultType]::ParameterName, 'Resume a specific session ID')
            [CompletionResult]::new('--log-level', '--log-level', [CompletionResultType]::ParameterName, 'Log level')
            [CompletionResult]::new('--log-format', '--log-format', [CompletionResultType]::ParameterName, 'Log format')
            [CompletionResult]::new('--config', '--config', [CompletionResultType]::ParameterName, 'Config file path')
        )
    }

    # Filter completions by the current word being typed
    $completions | Where-Object { $_.CompletionText -like "$wordToComplete*" -or $wordToComplete -eq '' }
}
