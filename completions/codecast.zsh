#compdef codecast
# ----------------------------------------------------------------------
# Zsh completion for codecast CLI
# ----------------------------------------------------------------------
# Installation:
#   Source this file directly:
#     source /path/to/codecast.zsh
#   Or copy to a zsh completion directory:
#     cp codecast.zsh ~/.zfunc/_codecast
#     fpath=(~/.zfunc $fpath)
# ----------------------------------------------------------------------

_codecast() {
    local -a global_flags
    global_flags=(
        '--model[AI model to use]:model:_codecast_models'
        '-m[AI model to use]:model:_codecast_models'
        '--provider[LLM provider]:provider:_codecast_providers'
        '-p[LLM provider]:provider:_codecast_providers'
        '--api-key[API key]:api_key:'
        '-k[API key]:api_key:'
        '--base-url[API base URL]:url:'
        '-u[API base URL]:url:'
        '--permission-mode[Permission approval mode]:mode:(suggest auto-edit full-auto)'
        '--scope[File access scope]:scope:_directories'
        '--safe[Safe mode - disable Shell and Web tools]'
        '--continue[Continue the most recent session]'
        '-c[Continue the most recent session]'
        '--resume[Resume a specific session ID]:session_id:'
        '-r[Resume a specific session ID]:session_id:'
        '--log-level[Log level]:level:(debug info warn error)'
        '--log-format[Log format]:format:(text json)'
        '--config[Config file path]:file:_files'
    )

    _arguments -C \
        $global_flags \
        '1:subcommand:_codecast_subcommands' \
        '*::arg:->args'

    case $state in
        args)
            case ${words[1]} in
                exec)
                    _codecast_exec
                    ;;
                init)
                    _codecast_init
                    ;;
                config)
                    _codecast_config
                    ;;
                plugin)
                    _codecast_plugin
                    ;;
                pool)
                    _codecast_pool
                    ;;
                sandbox)
                    _codecast_sandbox
                    ;;
                mcp)
                    _codecast_mcp
                    ;;
                cost)
                    _codecast_cost
                    ;;
            esac
            ;;
    esac
}

_codecast_subcommands() {
    local -a subcommands
    subcommands=(
        'exec:Headless mode - execute a single prompt'
        'init:Initialize project configuration'
        'config:Configuration management'
        'plugin:Manage community plugins'
        'pool:Manage Agent Pool'
        'sandbox:Sandbox isolation management'
        'mcp:MCP Server management'
        'cost:Cost tracking management'
    )
    _describe 'subcommand' subcommands
}

_codecast_models() {
    local -a models
    models=(
        'gpt-4o:GPT-4o'
        'gpt-4o-mini:GPT-4o Mini'
        'gpt-4-turbo:GPT-4 Turbo'
        'claude-3.5-sonnet:Claude 3.5 Sonnet'
        'claude-3-opus:Claude 3 Opus'
        'gemini-1.5-pro:Gemini 1.5 Pro'
        'deepseek-chat:DeepSeek Chat'
        'qwen-turbo:Qwen Turbo'
        'glm-4:GLM-4'
    )
    _describe 'model' models
}

_codecast_providers() {
    local -a providers
    providers=(
        'openai:OpenAI'
        'anthropic:Anthropic'
        'gemini:Google Gemini'
        'ollama:Ollama (local)'
        'deepseek:DeepSeek'
        'qwen:Qwen (Alibaba)'
        'glm:GLM (Zhipu AI)'
    )
    _describe 'provider' providers
}

_codecast_exec() {
    _arguments \
        '--format[Output format]:format:(text json stream-json)' \
        '-f[Output format]:format:(text json stream-json)' \
        '--model[Override model]:model:_codecast_models' \
        '-m[Override model]:model:_codecast_models' \
        '--timeout[Timeout in seconds]:seconds:' \
        '-t[Timeout in seconds]:seconds:' \
        '1:prompt:'
}

_codecast_init() {
    # No additional arguments
    _message 'No additional arguments'
}

_codecast_config() {
    local -a config_subcommands
    config_subcommands=(
        'set:Set a configuration value'
        'get:Get a configuration value'
        'list:List all configuration'
        'init:Initialize config file'
        'providers:List supported LLM providers'
    )

    if (( CURRENT == 2 )); then
        _describe 'config subcommand' config_subcommands
    elif (( CURRENT >= 3 )); then
        case ${words[2]} in
            set|get)
                if (( CURRENT == 3 )); then
                    local -a config_keys
                    config_keys=(
                        'api_key:API key'
                        'model:Default model'
                        'provider:Default provider'
                        'base_url:API base URL'
                    )
                    _describe 'config key' config_keys
                fi
                ;;
        esac
    fi
}

_codecast_plugin() {
    local -a plugin_subcommands
    plugin_subcommands=(
        'list:List installed plugins'
        'available:List available plugins'
        'install:Install a plugin'
        'unload:Unload a plugin'
    )

    _arguments \
        '--dir[Plugin install directory]:dir:_directories' \
        '1:subcommand:_describe "plugin subcommand" plugin_subcommands'
}

_codecast_pool() {
    local -a pool_subcommands
    pool_subcommands=(
        'status:View Pool status'
        'run:Submit parallel tasks'
    )

    _arguments \
        '--concurrency[Maximum concurrency]:count:' \
        '1:subcommand:_describe "pool subcommand" pool_subcommands'
}

_codecast_sandbox() {
    local -a sandbox_subcommands
    sandbox_subcommands=(
        'status:View sandbox status'
        'build:Build sandbox image'
    )

    if (( CURRENT == 2 )); then
        _describe 'sandbox subcommand' sandbox_subcommands
    fi
}

_codecast_mcp() {
    local -a mcp_subcommands
    mcp_subcommands=(
        'list:List registered MCP servers'
        'add:Register an MCP server'
        'remove:Remove an MCP server'
        'test:Test MCP server connection'
        'templates:List available MCP server templates'
        'categories:List MCP server categories'
    )

    if (( CURRENT == 2 )); then
        _describe 'mcp subcommand' mcp_subcommands
    elif (( CURRENT >= 3 )); then
        case ${words[2]} in
            add)
                _arguments \
                    '--command[Start command]:command:' \
                    '-c[Start command]:command:' \
                    '--args[Command arguments]:args:' \
                    '-a[Command arguments]:args:' \
                    '--url[Server URL]:url:' \
                    '-u[Server URL]:url:' \
                    '--auto-start[Auto start]:bool:(true false)' \
                    '1:name:'
                ;;
            remove|test)
                # Could complete registered MCP server names
                _message 'server name'
                ;;
        esac
    fi
}

_codecast_cost() {
    local -a cost_subcommands
    cost_subcommands=(
        'summary:View cost summary'
        'daily:View daily cost statistics'
        'list:View recent call records'
        'clear:Clear all cost records'
    )

    if (( CURRENT == 2 )); then
        _describe 'cost subcommand' cost_subcommands
    elif (( CURRENT >= 3 )); then
        case ${words[2]} in
            summary|daily|list)
                _arguments \
                    '--json[Output in JSON format]'
                ;;
            daily)
                _arguments \
                    '--json[Output in JSON format]' \
                    '1:days:'
                ;;
            list)
                _arguments \
                    '--json[Output in JSON format]' \
                    '1:limit:'
                ;;
        esac
    fi
}

_codecast "$@"
