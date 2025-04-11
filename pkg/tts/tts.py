import os
import sys
from dotenv import load_dotenv
from elevenlabs.client import ElevenLabs
from elevenlabs import play

def text_to_speech(text):
    load_dotenv()
    
    client = ElevenLabs(
        api_key=os.getenv("ELEVENLABS_API_KEY"),
    )
    
    audio = client.text_to_speech.convert(
        text=text,
        voice_id="JBFqnCBsd6RMkjVDRZzb",
        model_id="eleven_multilingual_v2",
        output_format="mp3_44100_128",
    )
    
    play(audio)

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Please provide text as an argument")
        sys.exit(1)
    
    text = sys.argv[1]
    text_to_speech(text) 