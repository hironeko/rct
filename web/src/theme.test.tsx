import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { ThemeProvider, useTheme } from "./theme";

describe("ThemeProvider", () => {
  beforeEach(() => window.localStorage.clear());
  afterEach(() => {
    cleanup();
    delete document.documentElement.dataset.theme;
    document.documentElement.style.removeProperty("color-scheme");
  });

  it("applies and persists dark, light, and half themes", () => {
    const view = render(<ThemeProvider initialTheme="dark"><ThemeProbe /></ThemeProvider>);
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");

    fireEvent.click(screen.getByRole("button", { name: "half" }));
    expect(document.documentElement).toHaveAttribute("data-theme", "half");
    expect(window.localStorage.getItem("rct.theme")).toBe("half");

    view.unmount();
    render(<ThemeProvider><ThemeProbe /></ThemeProvider>);
    expect(screen.getByTestId("current-theme")).toHaveTextContent("half");
  });
});

function ThemeProbe() {
  const { theme, setTheme } = useTheme();
  return <><span data-testid="current-theme">{theme}</span>{(["dark", "light", "half"] as const).map((option) => (
    <button type="button" key={option} onClick={() => setTheme(option)}>{option}</button>
  ))}</>;
}
