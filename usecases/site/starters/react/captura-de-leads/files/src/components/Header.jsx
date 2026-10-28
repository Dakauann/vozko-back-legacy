export default function Header() {
  return (
    <header className="sticky top-0 z-30 border-b border-line/70 bg-surface/85 backdrop-blur">
      <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
        <a href="/" className="flex items-center gap-2 font-display text-xl font-semibold text-ink">
          <span className="grid size-9 place-items-center rounded-xl bg-brand text-base text-brand-ink">S</span>
          Sua Empresa
        </a>
        <nav className="hidden items-center gap-8 text-sm font-medium text-muted md:flex">
          <a href="/#beneficios" className="hover:text-ink">Benefícios</a>
          <a href="/#depoimentos" className="hover:text-ink">Depoimentos</a>
          <a href="/#perguntas" className="hover:text-ink">Dúvidas</a>
        </nav>
        <a href="/#contato" data-vz-cta="topo" className="rounded-full bg-ink px-5 py-2.5 text-sm font-semibold text-surface transition hover:bg-brand">Agendar</a>
      </div>
    </header>
  );
}
