#!/bin/bash
# ----------------------------------------------------------------------
# Bash completion for codecast CLI
# ----------------------------------------------------------------------
# Installation:
#   Source this file directly:
#     source /path/to/codecast.bash
#   Or copy to a bash-completion directory:
#     cp codecast.bash /etc/bash_completion.d/
#     cp codecast.bash ~/.local/share/bash-completion/completions/codecast
# ----------------------------------------------------------------------

_codecast() {
    local cur prev words cword
    _init_completion || return

    # Global flags that take an argument
    local global_flags_with_args=(
        --model -m
        --provider -p
        --api-key -k
        --base-url -u
        --permission-mode
        --scope
        --log-level
        --log-format
        --resume -r
        --config
    )

    # Global boolean flags
    local global_flags_bool=(
        --safe
        --continue -c
    )

    # Top-level subcommands
    local subcommands=(
        exec
        init
        config
        plugin
        pool
        sandbox
        mcp
        cost
    )

    # Subcommand completions
    local config_subcommands="set get list init providers"
    local plugin_subcommands="list available install unload"
    local pool_subcommands="status run"
    local sandbox_subcommands="status build"
    local mcp_subcommands="list add remove test templates categories"
    local cost_subcommands="summary daily list clear"

    # Config keys for 'config set' and 'config get'
    local config_keys="api_key model provider base_url"

    # Provider values
    local providers="openai anthropic gemini ollama deepseek qwen glm"

    # Permission-mode values
    local permission_modes="suggest auto-edit full-auto"

    # Log-level values
    local log_levels="debug info warn error"

    # Log-format values
    local log_formats="text json"

    # Exec format values
    local exec_formats="text json stream-json"

    # Interactive slash commands (for context, not directly completed in shell)
    local slash_commands="/help /h /quit /q /exit /clear /compact /rules /tools /models /stats /sessions /export /resume /plan /delegate /hooks /vision /screenshot /pool /plugins /index /model /sandbox /mcp /mode /undo /budget"

    # Determine which subcommand is active
    local subcommand=""
    local subcommand_index=""
    for ((i = 1; i < ${#words[@]}; i++)); do
        if [[ ${words[i]} != -* ]]; then
            subcommand="${words[i]}"
            subcommand_index=$i
            break
        fi
    done

    # If no subcommand yet, complete subcommands and global flags
    if [[ -z "$subcommand" ]]; then
        if [[ "$cur" == --* ]]; then
            COMPREPLY=($(compgen -W "${global_flags_with_args[*]} ${global_flags_bool[*]}" -- "$cur"))
            return
        elif [[ "$cur" == -* ]]; then
            COMPREPLY=($(compgen -W "${global_flags_with_args[*]} ${global_flags_bool[*]}" -- "$cur"))
            return
        else
            COMPREPLY=($(compgen -W "${subcommands[*]}" -- "$cur"))
            return
        fi
    fi

    # Complete arguments for global flags regardless of subcommand
    case "$prev" in
        --model|-m)
            # Suggest common model names
            COMPREPLY=($(compgen -W "gpt-4o gpt-4o-mini gpt-4-turbo claude-3.5-sonnet claude-3-opus gemini-1.5-pro deepseek-chat qwen-turbo glm-4" -- "$cur"))
            return
            ;;
        --provider|-p)
            COMPREPLY=($(compgen -W "$providers" -- "$cur"))
            return
            ;;
        --api-key|-k)
            # Don't complete API keys
            return
            ;;
        --base-url|-u)
            # Don't complete URLs
            return
            ;;
        --permission-mode)
            COMPREPLY=($(compgen -W "$permission_modes" -- "$cur"))
            return
            ;;
        --scope)
            # Suggest directories
            COMPREPLY=($(compgen -d -- "$cur"))
            return
            ;;
        --log-level)
            COMPREPLY=($(compgen -W "$log_levels" -- "$cur"))
            return
            ;;
        --log-format)
            COMPREPLY=($(compgen -W "$log_formats" -- "$cur"))
            return
            ;;
        --resume|-r)
            # Could list session IDs; leave empty for now
            return
            ;;
        --config)
            COMPREPLY=($(compgen -f -- "$cur"))
            return
            ;;
    esac

    # Subcommand-specific completion
    case "$subcommand" in
        exec)
            # exec-specific flags
            case "$prev" in
                --format|-f)
                    COMPREPLY=($(compgen -W "$exec_formats" -- "$cur"))
                    return
                    ;;
                --model|-m)
                    COMPREPLY=($(compgen -W "gpt-4o gpt-4o-mini claude-3.5-sonnet gemini-1.5-pro deepseek-chat qwen-turbo glm-4" -- "$cur"))
                    return
                    ;;
                --timeout|-t)
                    return
                    ;;
            esac
            if [[ "$cur" == -* ]]; then
                COMPREPLY=($(compgen -W "--format -f --model -m --timeout -t" -- "$cur"))
            fi
            return
            ;;
        init)
            # No subcommands or flags for init
            return
            ;;
        config)
            # Determine config subcommand
            local config_sub=""
            for ((i = subcommand_index + 1; i < ${#words[@]}; i++)); do
                if [[ ${words[i]} != -* ]]; then
                    config_sub="${words[i]}"
                    break
                fi
            done

            if [[ -z "$config_sub" && "$cur" != -* ]]; then
                COMPREPLY=($(compgen -W "$config_subcommands" -- "$cur"))
                return
            fi

            case "$config_sub" in
                set|get)
                    if [[ "$prev" == "$config_sub" || "$prev" == "config" ]]; then
                        COMPREPLY=($(compgen -W "$config_keys" -- "$cur"))
                        return
                    fi
                    ;;
            esac
            return
            ;;
        plugin)
            local plugin_sub=""
            for ((i = subcommand_index + 1; i < ${#words[@]}; i++)); do
                if [[ ${words[i]} != -* ]]; then
                    plugin_sub="${words[i]}"
                    break
                fi
            done

            if [[ -z "$plugin_sub" && "$cur" != -* ]]; then
                COMPREPLY=($(compgen -W "$plugin_subcommands" -- "$cur"))
                return
            fi

            case "$prev" in
                --dir)
                    COMPREPLY=($(compgen -d -- "$cur"))
                    return
                    ;;
            esac

            if [[ "$cur" == -* ]]; then
                COMPREPLY=($(compgen -W "--dir" -- "$cur"))
            fi
            return
            ;;
        pool)
            local pool_sub=""
            for ((i = subcommand_index + 1; i < ${#words[@]}; i++)); do
                if [[ ${words[i]} != -* ]]; then
                    pool_sub="${words[i]}"
                    break
                fi
            done

            if [[ -z "$pool_sub" && "$cur" != -* ]]; then
                COMPREPLY=($(compgen -W "$pool_subcommands" -- "$cur"))
                return
            fi

            case "$prev" in
                --concurrency)
                    return
                    ;;
            esac

            if [[ "$cur" == -* ]]; then
                COMPREPLY=($(compgen -W "--concurrency" -- "$cur"))
            fi
            return
            ;;
        sandbox)
            local sandbox_sub=""
            for ((i = subcommand_index + 1; i < ${#words[@]}; i++)); do
                if [[ ${words[i]} != -* ]]; then
                    sandbox_sub="${words[i]}"
                    break
                fi
            done

            if [[ -z "$sandbox_sub" && "$cur" != -* ]]; then
                COMPREPLY=($(compgen -W "$sandbox_subcommands" -- "$cur"))
                return
            fi
            return
            ;;
        mcp)
            local mcp_sub=""
            for ((i = subcommand_index + 1; i < ${#words[@]}; i++)); do
                if [[ ${words[i]} != -* ]]; then
                    mcp_sub="${words[i]}"
                    break
                fi
            done

            if [[ -z "$mcp_sub" && "$cur" != -* ]]; then
                COMPREPLY=($(compgen -W "$mcp_subcommands" -- "$cur"))
                return
            fi

            case "$mcp_sub" in
                add)
                    case "$prev" in
                        --command|-c)
                            return
                            ;;
                        --args|-a)
                            return
                            ;;
                        --url|-u)
                            return
                            ;;
                        --auto-start)
                            COMPREPLY=($(compgen -W "true false" -- "$cur"))
                            return
                            ;;
                    esac
                    if [[ "$cur" == -* ]]; then
                        COMPREPLY=($(compgen -W "--command -c --args -a --url -u --auto-start" -- "$cur"))
                    fi
                    return
                    ;;
            esac
            return
            ;;
        cost)
            local cost_sub=""
            for ((i = subcommand_index + 1; i < ${#words[@]}; i++)); do
                if [[ ${words[i]} != -* ]]; then
                    cost_sub="${words[i]}"
                    break
                fi
            done

            if [[ -z "$cost_sub" && "$cur" != -* ]]; then
                COMPREPLY=($(compgen -W "$cost_subcommands" -- "$cur"))
                return
            fi

            case "$cost_sub" in
                summary|daily|list)
                    if [[ "$cur" == -* ]]; then
                        COMPREPLY=($(compgen -W "--json" -- "$cur"))
                    fi
                    return
                    ;;
            esac
            return
            ;;
    esac
}

complete -F _codecast codecast
