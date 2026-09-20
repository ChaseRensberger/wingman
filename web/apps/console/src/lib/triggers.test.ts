import { expect, test } from "bun:test";
import { describeSchedule, scheduledTime } from "./triggers";

test("describes known cron patterns without misrepresenting custom schedules", () => {
  expect(describeSchedule("0 8 * * 1-5")).toBe("Weekdays at 8 AM");
  expect(describeSchedule("30 2 * * *")).toBe("Every day at 2:30 AM");
  expect(describeSchedule("0 12 * * 0")).toBe("Sundays at 12:00 PM");
  expect(describeSchedule("*/15 * * * *")).toBe("Every 15 minutes");
  expect(describeSchedule("*/7 * * * *")).toBe("Custom schedule");
  expect(describeSchedule("0 8 1 * 1")).toBe("Custom schedule");
});

test("schedule timestamps use the trigger zone, including daylight saving", () => {
  const date = "2026-09-16T12:00:00Z";
  expect(scheduledTime(date, "America/New_York")).toBe(
    new Intl.DateTimeFormat(undefined, {
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
      timeZone: "America/New_York",
    }).format(new Date(date)),
  );
});
