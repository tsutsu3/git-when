import { describe, expect, it } from "vitest";

import { MESSAGES, dayName, isLang, translate, type MessageTable } from "./i18n";

describe("translate", () => {
  it("returns the message in the language", () => {
    expect(translate("en", "reset")).toBe("Clear filters");
    expect(translate("ja", "reset")).toBe("条件をクリア");
  });

  it("fills placeholders", () => {
    expect(translate("en", "weekendHead", { days: "Sat, Sun", n: 2 })).toBe(
      "Weekend: Sat, Sun (2 days)",
    );
  });

  it("leaves missing placeholders empty", () => {
    expect(translate("en", "weekendHead", { n: 2 })).toBe("Weekend:  (2 days)");
  });

  it("falls back to English, then to the key", () => {
    const messages: MessageTable = { en: { only: "English only" }, ja: {} };
    expect(translate("ja", "only", undefined, messages)).toBe("English only");
    expect(translate("ja", "missing", undefined, messages)).toBe("missing");
  });

  it("has the same keys in every language", () => {
    expect(Object.keys(MESSAGES.ja).sort()).toEqual(Object.keys(MESSAGES.en).sort());
  });
});

describe("language helpers", () => {
  it("accepts only supported languages", () => {
    expect(isLang("en")).toBe(true);
    expect(isLang("ja")).toBe(true);
    expect(isLang("fr")).toBe(false);
    expect(isLang(null)).toBe(false);
  });

  it("names weekdays", () => {
    expect(dayName("en", 0)).toBe("Mon");
    expect(dayName("ja", 6)).toBe("日");
  });
});
