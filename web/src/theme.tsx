import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

export type Theme = "dark" | "light" | "half";

interface ThemeValue {
  theme: Theme;
  setTheme: (theme: Theme) => void;
}

const ThemeContext = createContext<ThemeValue | undefined>(undefined);

export function ThemeProvider({ children, initialTheme }: { children: ReactNode; initialTheme?: Theme }) {
  const [theme, setThemeState] = useState<Theme>(() => initialTheme ?? detectedTheme());

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme === "dark" ? "dark" : "light";
  }, [theme]);

  const value = useMemo<ThemeValue>(() => ({
    theme,
    setTheme: (next) => {
      setThemeState(next);
      try { window.localStorage.setItem("rct.theme", next); } catch { /* visual preference is optional */ }
    },
  }), [theme]);

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeValue {
  const value = useContext(ThemeContext);
  if (!value) throw new Error("useTheme must be used within ThemeProvider");
  return value;
}

function detectedTheme(): Theme {
  try {
    const saved = window.localStorage.getItem("rct.theme");
    if (saved === "dark" || saved === "light" || saved === "half") return saved;
  } catch { /* retain the dark default */ }
  return "dark";
}
