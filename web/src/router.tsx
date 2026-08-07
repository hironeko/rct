import { createBrowserRouter } from "react-router";
import { AppShell } from "./routes/AppShell";
import { Home } from "./routes/Home";
import { RunDetail } from "./routes/RunDetail";

export const router = createBrowserRouter(
  [
    {
      path: "/",
      element: <AppShell />,
      errorElement: <AppShell error />,
      children: [
        { index: true, element: <Home /> },
        { path: "runs/:runId", element: <RunDetail /> },
      ],
    },
  ],
  { basename: "/ui" },
);
