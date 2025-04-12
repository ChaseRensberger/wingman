import os
from elevenlabs import ElevenLabs
from elevenlabs import play

class ElevenLabsClient:
    def __init__(self):
        self.client = ElevenLabs(
            api_key=os.getenv("ELEVENLABS_API_KEY"),
        )
        
    def text_to_speech(self, text):
        audio = self.client.text_to_speech.convert(
            text=text,
            voice_id="JBFqnCBsd6RMkjVDRZzb",
            model_id="eleven_multilingual_v2",
            output_format="mp3_44100_128",
        )
        return audio
    
    def play_audio(self, audio):
        play(audio)