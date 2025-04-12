import logging
from llm import AnthropicClient
from tts import ElevenLabsClient
from dotenv import load_dotenv

load_dotenv()

logger = logging.getLogger(__name__)

def loop():
    anthropic_client = AnthropicClient()
    elevenlabs_client = ElevenLabsClient()
    while True:
        input_text = input("Enter a text: ")
        if input_text == "exit":
            print(anthropic_client.conversation_history)
            break
        output = anthropic_client.generate_message(input_text)
        print(output)
        # speech = elevenlabs_client.text_to_speech(completion)
        # elevenlabs_client.play_audio(speech)

if __name__ == "__main__":
    loop()