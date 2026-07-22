const DISPLAY_NAME_KEY = "wingman_display_name";

const generalGreetings = [
  "{name} returns!",
  "Back at it, {name}",
  "Back at it!",
  "Greetings, whoever you are",
  "Hey there",
  "Hey there, {name}",
  "Hi {name}, how are you?",
  "Hi, how are you?",
  "How's it going, {name}?",
  "How's it going?",
  "Let's chat incognito",
  "Welcome",
  "Welcome, {name}",
  "What's new, {name}?",
  "What's new?",
  "What's on your mind, {name}?",
  "What's on your mind?",
  "You're incognito",
];

const dayGreetings = [
  ["Happy Sunday", "Happy Sunday, {name}", "Sunday session?", "Sunday session, {name}?"],
  ["Happy Monday", "Happy Monday, {name}"],
  ["Happy Tuesday", "Happy Tuesday, {name}"],
  ["Happy Wednesday", "Happy Wednesday, {name}"],
  ["Happy Thursday", "Happy Thursday, {name}"],
  ["Happy Friday", "Happy Friday, {name}", "That Friday feeling", "That Friday feeling, {name}"],
  ["Happy Saturday!", "Happy Saturday, {name}", "Welcome to the weekend", "Welcome to the weekend, {name}"],
];

function timeGreetings(hour: number): string[] {
  if (hour < 5 || hour >= 21) {
    return ["Hello, night owl"];
  }
  if (hour < 12) {
    return ["Coffee and Claude time?", "Good morning", "Good morning, {name}"];
  }
  if (hour < 17) {
    return ["Good afternoon", "Good afternoon, {name}"];
  }
  return [
    "Evening",
    "Evening, {name}",
    "Good evening",
    "Good evening, {name}",
    "How was your day, {name}?",
    "How was your day?",
    "What's on your mind tonight?",
    "What's on your mind tonight, {name}?",
  ];
}

export function getDisplayName(): string {
  return localStorage.getItem(DISPLAY_NAME_KEY)?.trim() ?? "";
}

export function setDisplayName(name: string) {
  const trimmed = name.trim();
  if (trimmed) {
    localStorage.setItem(DISPLAY_NAME_KEY, trimmed);
  } else {
    localStorage.removeItem(DISPLAY_NAME_KEY);
  }
}

export function selectGreeting(name = getDisplayName(), now = new Date()): string {
  const groups = [dayGreetings[now.getDay()]!, timeGreetings(now.getHours()), generalGreetings]
    .map((greetings) => greetings.filter((greeting) => name || !greeting.includes("{name}")));
  const candidates = groups[Math.floor(Math.random() * groups.length)]!;
  const greeting = candidates[Math.floor(Math.random() * candidates.length)]!;
  return greeting.replaceAll("{name}", name);
}
