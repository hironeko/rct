import { createRoot } from "react-dom/client";
import { RouterProvider } from "react-router/dom";
import { bootstrapSession } from "./api/client";
import { router } from "./router";
import "./styles/app.css";

const rootElement = document.getElementById("root");
if (!rootElement) throw new Error("rct UI root element is missing");

const root = createRoot(rootElement);

bootstrapSession()
  .then(() => root.render(<RouterProvider router={router} />))
  .catch((error: unknown) => {
    const message = error instanceof Error ? error.message : "The local session could not be established";
    root.render(
      <main className="session-error">
        <p className="eyebrow">LOCAL SESSION REQUIRED</p>
        <h1>Unable to open rct</h1>
        <p>{message}</p>
        <p>Return to the terminal and restart <code>rct serve</code>.</p>
      </main>,
    );
  });
