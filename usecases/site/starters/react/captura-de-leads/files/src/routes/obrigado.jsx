import { CircleCheck, MessageCircle } from "lucide-react";
import Footer from "../components/Footer.jsx";
import Header from "../components/Header.jsx";

export default function Obrigado() {
  return (
    <>
      <Header />
      <main className="bg-surface font-sans text-ink">
        <section className="mx-auto flex min-h-[70vh] max-w-2xl flex-col items-center justify-center px-6 py-24 text-center">
          <span className="grid size-16 place-items-center rounded-full bg-brand text-brand-ink">
            <CircleCheck className="size-8" />
          </span>
          <h1 className="mt-8 font-display text-4xl font-semibold text-balance md:text-5xl">Recebemos seu contato</h1>
          <p className="mt-4 text-lg leading-relaxed text-pretty text-muted">Em até 1 dia útil um especialista chama você no WhatsApp para combinar o melhor horário. Se preferir, fale com a gente agora.</p>
          <a href="#" data-vz-whatsapp="" data-vz-cta="whatsapp-obrigado" className="mt-10 inline-flex items-center gap-2 rounded-full bg-[#25D366] px-7 py-3.5 text-base font-semibold text-white shadow-lg shadow-[#25D366]/25 transition hover:-translate-y-0.5">
            <MessageCircle className="size-5" />
            Falar no WhatsApp agora
          </a>
          <a href="/" className="mt-6 text-sm font-medium text-muted underline">Voltar para o início</a>
        </section>
      </main>
      <Footer />
    </>
  );
}
