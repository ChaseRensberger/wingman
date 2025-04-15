import subprocess
import json

with open("mcp.config.json", "r") as f:
    config = json.load(f)

processes = []

for name, server in config["mcpServers"].items():
    cmd = [server["command"]] + server["args"]
    print(f"Starting {name} server with command: {' '.join(cmd)}")

    try:
        proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, universal_newlines=True)
        processes.append((name, proc))
    except Exception as e:
        print(f"Error starting {name} server: {e}")
        exit(1)

print("All servers started successfully.")

