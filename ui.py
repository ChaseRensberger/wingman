from textual.app import App, ComposeResult
from textual.widgets import Footer, Header, Static
from textual.screen import Screen
from textual.widget import Widget
from textual.reactive import reactive
from textual import work
import math
import asyncio

class CircularPulse(Widget):
    """A widget that displays an animated circular pulse."""
    
    DEFAULT_CSS = """
    CircularPulse {
        width: 100%;
        height: 100%;
        content-align: center middle;
    }
    """
    
    radius = reactive(0)
    max_radius = 10
    pulse_speed = 0.1
    
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.animation_task = None
    
    def on_mount(self) -> None:
        """Start the animation when the widget is mounted."""
        self.animation_task = self.animate_pulse()
    
    def on_unmount(self) -> None:
        """Stop the animation when the widget is unmounted."""
        if self.animation_task:
            self.animation_task.cancel()
    
    @work
    async def animate_pulse(self):
        """Animate the pulse effect."""
        while True:
            self.radius = (self.radius + self.pulse_speed) % self.max_radius
            self.refresh()
            await asyncio.sleep(0.05)
    
    def render(self) -> str:
        """Render the circular pulse."""
        size = self.max_radius * 2 + 1
        grid = [[' ' for _ in range(size)] for _ in range(size)]
        
        center_x = center_y = self.max_radius
        
        for y in range(size):
            for x in range(size):
                dx = x - center_x
                dy = y - center_y
                distance = math.sqrt(dx*dx + dy*dy)
                
                if abs(distance - self.radius) < 1:
                    grid[y][x] = '●'
                elif abs(distance - self.radius) < 2:
                    grid[y][x] = '○'
                elif abs(distance - self.radius) < 3:
                    grid[y][x] = '·'
        
        return '\n'.join(''.join(row) for row in grid)

class Log(Screen):
    def compose(self) -> ComposeResult:
        yield Header()
        yield Static(" Log Screen ", id="title")
        yield Footer()

class Visual(Screen):
    DEFAULT_CSS = """
    Visual {
        align: center middle;
    }
    """
    
    def compose(self) -> ComposeResult:
        yield Header()
        yield CircularPulse()
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


