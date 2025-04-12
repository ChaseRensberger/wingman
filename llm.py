import anthropic
import os
from tools import get_weather

tools = {
    "get_weather": get_weather
}

class AnthropicClient:
    def __init__(self):
        self.tools = [
            {
                "type": "custom",
                "name": "get_weather",
                "description": "can get the weather",
                "input_schema": {
                "type": "object",
                "properties": {
                    "location": {
                        "type": "string",
                        "description": "The city and state, e.g. San Francisco, CA"
                    }
                },
                "required": [
                    "location"
                ]
                }
            }
        ]
        self.client = anthropic.Anthropic(
            api_key=os.getenv("ANTHROPIC_API_KEY"),
        )
        self.conversation_history = []
        
    def generate_message(self, input_text):
        self.conversation_history.append({
            "role": "user",
            "content": input_text
        })
        
        response = self.client.messages.create(
            model="claude-3-7-sonnet-20250219",
            max_tokens=20000,
            temperature=1,
            system="You are a helpful assistant",
            messages=self.conversation_history,
            tools=self.tools
        )
        
        self.conversation_history.append({
            "role": "assistant",
            "content": response.content
        })
        
        return self.process_message(response)
    
    def process_message(self, response):
        tool_use_blocks = [block for block in response.content if block.type == "tool_use"]
        
        if not tool_use_blocks:
            text_blocks = [block.text for block in response.content if block.type == "text"]
            return "".join(text_blocks)
        
        for tool_use_block in tool_use_blocks:
            tool_name = tool_use_block.name
            tool_input = tool_use_block.input
            
            tool_result = self.get_tool_result(tool_name, tool_input)
            
            self.conversation_history.append({
                "role": "user",
                "content": [
                    {
                        "type": "tool_result",
                        "tool_use_id": tool_use_block.id,
                        "content": tool_result
                    }
                ]
            })
        
        follow_up_response = self.client.messages.create(
            model="claude-3-7-sonnet-20250219",
            max_tokens=20000,
            temperature=1,
            system="You are a helpful assistant",
            messages=self.conversation_history,
            tools=self.tools
        )
        
        self.conversation_history.append({
            "role": "assistant",
            "content": follow_up_response.content
        })
        
        text_blocks = [block.text for block in follow_up_response.content if block.type == "text"]
        return "".join(text_blocks)

    def get_tool_result(self, tool_name, tool_input=None):
        if tool_name in tools:
            if tool_input:
                if tool_name == "get_weather" and "location" in tool_input:
                    result = tools[tool_name](location=tool_input["location"])
                else:
                    result = tools[tool_name](**tool_input)
            else:
                result = tools[tool_name]()
            
            if isinstance(result, (int, float)):
                return str(result)
            return result
        else:
            return "Tool not found"
