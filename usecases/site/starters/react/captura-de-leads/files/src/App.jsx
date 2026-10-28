import Home from "./routes/index.jsx";
import Obrigado from "./routes/obrigado.jsx";
import Privacidade from "./routes/privacidade.jsx";

const routes = { "/": Home, "/obrigado": Obrigado, "/privacidade": Privacidade };

export default function App() {
  const path = typeof window === "undefined" ? "/" : window.location.pathname.replace(/\/+$/, "") || "/";
  const Page = routes[path] ?? Home;
  return <Page />;
}
