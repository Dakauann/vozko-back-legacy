export default function Home() {
  return (
    <main className="min-h-screen bg-surface font-sans text-ink">
      <section className="mx-auto flex min-h-screen max-w-3xl flex-col items-center justify-center px-6 text-center">
        <span className="rounded-full bg-brand/10 px-4 py-1 text-sm font-medium text-brand">Sua nova página</span>
        <h1 className="mt-6 font-display text-5xl leading-tight text-balance md:text-6xl">Comece a contar a sua história aqui</h1>
        <p className="mt-6 max-w-xl text-lg text-pretty text-muted">Troque este texto pelo que você oferece. Clique em qualquer parte para editar ou peça para a Elo montar as seções.</p>
        <a href="#" data-vz-cta="principal" className="mt-10 inline-flex items-center rounded-full bg-brand px-8 py-4 text-base font-semibold text-brand-ink shadow-lg shadow-brand/20 transition hover:-translate-y-0.5">Quero saber mais</a>
      </section>
    </main>
  );
}
