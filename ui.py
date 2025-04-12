from textual.app import App, ComposeResult
from textual.widgets import Footer, Header, Static
from textual.screen import Screen

class Log(Screen):
    def compose(self) -> ComposeResult:
        yield Header()
        yield Static(" Log Screen ", id="title")
        yield Footer()

class Visual(Screen):
    def compose(self) -> ComposeResult:
        yield Header()
        yield Static(" Visual Screen ", id="title")
        yield Footer()

class Isaac(App):
    SCREENS = { "visual": Visual, "log": Log }
    BINDINGS = [
                  ("d", "toggle_dark", "Toggle dark mode"),
                  ("v", "switch_screen('visual')", "Show visual screen"),
                  ("l", "switch_screen('log')", "Show log screen"),
               ]

    def compose(self) -> ComposeResult:
        """Create child widgets for the app."""
        yield Header()
        yield Static(" Main Screen ", id="title")
        yield Footer()
    
    def action_toggle_dark(self) -> None:
        """An action to toggle dark mode."""
        self.theme = (
            "textual-dark" if self.theme == "textual-light" else "textual-light"
        )
    
    def on_mount(self) -> None:
        """Set up the app when it starts."""
        self.title = "Isaac"
        self.push_screen("visual")

if __name__ == "__main__":
    Isaac().run()


