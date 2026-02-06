# Agents

[Source](../agent/agent.go)

The most essential primitive of Wingman is the agent. An Agent is stateless template that defines *how* to process some unit of work.

## Endpoints

**Example Request:**
```bash
curl -X POST http://localhost:8080/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "transaction-categorizer",
    "instructions": "You are a financial assistant that categorizes transactions. Given a list of transactions, return each transaction with an appropriate category such as: groceries, dining, entertainment, utilities, transportation, shopping, healthcare, or other.",
    "max_tokens": 2048,
    "max_steps": 1,
    "output_schema": {
      "type": "object",
      "properties": {
        "transactions": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "id": { "type": "string" },
              "description": { "type": "string" },
              "amount": { "type": "number" },
              "category": { "type": "string" }
            }
          }
        }
      }
    }
  }'
```

**Example Response:**
```json
{
  "id": "01HQXYZ...",
  "name": "transaction-categorizer",
  "instructions": "You are a financial assistant that categorizes transactions. Given a list of transactions, return each transaction with an appropriate category such as: groceries, dining, entertainment, utilities, transportation, shopping, healthcare, or other.",
  "max_tokens": 2048,
  "max_steps": 1,
  "output_schema": {
    "type": "object",
    "properties": {
      "transactions": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "id": { "type": "string" },
            "description": { "type": "string" },
            "amount": { "type": "number" },
            "category": { "type": "string" }
          }
        }
      }
    }
  },
  "created_at": "2026-01-15T10:30:00Z",
  "updated_at": "2026-01-15T10:30:00Z"
}
```
