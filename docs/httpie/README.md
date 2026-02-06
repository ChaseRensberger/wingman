# HTTPie Request Files

This directory contains `.http` files for all Wingman API endpoints. You can run these files with the [HTTPie](https://httpie.io/) CLI to experiment with them.

## Variables

The files use `{{variable}}` syntax for dynamic values. Set these before running requests:

- `{{agent_id}}` - Agent ID returned from POST /agents
- `{{session_id}}` - Session ID returned from POST /sessions
- `{{fleet_id}}` - Fleet ID returned from POST /fleets
- `{{formation_id}}` - Formation ID returned from POST /formations
- `{{researcher_agent_id}}` - Agent ID for researcher role
- `{{writer_agent_id}}` - Agent ID for writer role

## Quick Start

```bash
# Start the server
wingman serve

# Set auth (replace with your API key)
http PUT localhost:2323/providers/auth providers:='{"anthropic":{"type":"api_key","key":"sk-ant-..."}}'

# Create an agent
http POST localhost:2323/agents name="assistant" instructions="You are helpful." tools:='["bash","read"]'

# Create a session
http POST localhost:2323/sessions work_dir="/tmp"

# Send a message
http POST localhost:2323/sessions/{session_id}/message agent_id="{agent_id}" prompt="Hello!"
```
