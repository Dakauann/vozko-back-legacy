import Home from "./routes/index.jsx";

const routes = { "/": Home };

export default function App() {
  const path = typeof window === "undefined" ? "/" : window.location.pathname.replace(/\/+$/, "") || "/";
  const Page = routes[path] ?? Home;
  return <Page />;
}
