from enum import Enum, auto

from textual.app import App, ComposeResult
from textual.containers import VerticalScroll
from textual.widgets import Footer, Header, Input, Markdown


class MessageType(Enum):
    User = auto()
    Assistant = auto()


def create_message(user_message: str):

    EXAMPLE_LLM_RESPONSE = """
    A Python for loop iterates over a sequence:\n\n```python\nfor item in sequence:\n    # do something with item\n```\n\n**Examples:**\n```python\n# Loop through a list\nfor name in ['Alice', 'Bob', 'Charlie']:\n    print(name)\n\n# Loop through a range of numbers\nfor i in range(5):  # 0, 1, 2, 3, 4\n    print(i)\n\n# Loop through a string\nfor letter in 'hello':\n    print(letter)\n```
    """
    return EXAMPLE_LLM_RESPONSE


class WingmanApp(App):
    def compose(self) -> ComposeResult:
        yield Header()
        yield VerticalScroll(id="message_output")
        yield Input(placeholder="Query", id="message_input")
        yield Footer()

    async def on_mount(self) -> None:
        self.query_one("#message_input", Input).focus()

    async def on_input_submitted(self, event: Input.Submitted) -> None:
        user_message = event.value.strip()
        if not user_message:
            return
        self.add_message_to_display(MessageType.User, user_message)

        self.clear_message_input()

        assistant_message = create_message(user_message)

        self.add_message_to_display(MessageType.Assistant, assistant_message)

    def add_message_to_display(self, type, message: str) -> None:
        message_output = self.query_one("#message_output")
        message_output.mount(Markdown(message))
        message_output.scroll_end(animate=True)

    def clear_message_input(self) -> None:
        message_input = self.query_one("#message_input", Input)
        message_input.value = ""
        message_input.focus()


def main():
    WingmanApp().run()


if __name__ == "__main__":
    main()
