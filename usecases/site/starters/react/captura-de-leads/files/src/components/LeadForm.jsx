export default function LeadForm() {
  return (
    <div id="contato" className="rounded-card border border-line bg-white p-8 shadow-2xl shadow-ink/10">
      <h2 className="font-display text-2xl font-semibold">Quero minha avaliação</h2>
      <p className="mt-2 text-sm text-muted">Leva menos de um minuto. Retornamos pelo WhatsApp.</p>
      <form data-vz-form="contato" className="mt-6 grid gap-4">
        <label className="grid gap-1.5 text-sm font-medium">
          Nome
          <input type="text" name="lead.name" required autoComplete="name" placeholder="Como podemos te chamar?" className="rounded-xl border border-line bg-surface px-4 py-3 text-base outline-none transition focus:border-brand focus:ring-4 focus:ring-brand/15" />
        </label>
        <label className="grid gap-1.5 text-sm font-medium">
          WhatsApp
          <input type="tel" name="lead.phone" required autoComplete="tel" placeholder="(11) 90000-0000" className="rounded-xl border border-line bg-surface px-4 py-3 text-base outline-none transition focus:border-brand focus:ring-4 focus:ring-brand/15" />
        </label>
        <label className="grid gap-1.5 text-sm font-medium">
          E-mail
          <input type="email" name="lead.email" autoComplete="email" placeholder="voce@email.com" className="rounded-xl border border-line bg-surface px-4 py-3 text-base outline-none transition focus:border-brand focus:ring-4 focus:ring-brand/15" />
        </label>
        <label className="flex items-start gap-3 text-sm leading-relaxed text-muted">
          <input type="checkbox" name="consent" required className="mt-1 size-4 accent-brand" />
          <span>
            Autorizo o contato por WhatsApp, telefone ou e-mail e concordo com a <a href="/privacidade" className="font-medium text-brand underline">Política de Privacidade</a>.
          </span>
        </label>
        <button type="submit" className="mt-2 rounded-xl bg-brand px-6 py-4 text-base font-semibold text-brand-ink transition hover:brightness-110">Agendar minha avaliação</button>
      </form>
    </div>
  );
}
