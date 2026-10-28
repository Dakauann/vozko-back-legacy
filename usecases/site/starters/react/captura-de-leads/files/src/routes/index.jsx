import { CalendarCheck, Check, HeartHandshake, MessageCircle, Route } from "lucide-react";
import Footer from "../components/Footer.jsx";
import Header from "../components/Header.jsx";
import LeadForm from "../components/LeadForm.jsx";

const promises = ["Sem compromisso e sem custo", "Atendimento presencial ou online", "Resposta em até 1 dia útil"];

const benefits = [
  { title: "Atendimento de verdade", text: "Você fala com quem entende do assunto desde o primeiro contato, sem robô e sem fila." },
  { title: "Plano sob medida", text: "Cada proposta parte do seu objetivo, do seu prazo e do seu orçamento." },
  { title: "Acompanhamento próximo", text: "Você recebe notícias a cada etapa e sabe exatamente o que vem a seguir." },
];

const testimonials = [
  { quote: "Saí da avaliação sabendo exatamente o que fazer. Em dois meses, o resultado apareceu.", author: "Mariana Costa, cliente desde 2024" },
  { quote: "Atendimento rápido, atencioso e sem enrolação. Recomendo para todo mundo.", author: "Rafael Souza, cliente desde 2023" },
];

const questions = [
  { question: "A avaliação é mesmo gratuita?", answer: "Sim. A primeira conversa não tem custo e você decide depois, sem pressão." },
  { question: "Quanto tempo leva?", answer: "Cerca de 30 minutos, presencial ou por videochamada, no horário que for melhor para você." },
  { question: "Como vocês entram em contato?", answer: "Pelo WhatsApp informado no formulário, em até 1 dia útil, para combinar o melhor horário." },
];

const benefitIcons = [HeartHandshake, Route, CalendarCheck];

export default function Home() {
  return (
    <>
      <Header />
      <main className="bg-surface font-sans text-ink">
        <section className="relative overflow-hidden">
          <div className="pointer-events-none absolute -top-40 -left-32 size-[34rem] rounded-full bg-brand/15 blur-3xl" />
          <div className="pointer-events-none absolute -top-24 right-0 size-[26rem] rounded-full bg-accent/25 blur-3xl" />
          <div className="relative mx-auto grid max-w-6xl gap-12 px-6 pt-16 pb-20 md:grid-cols-[1.1fr_0.9fr] md:items-center md:pt-24">
            <div>
              <span className="inline-flex items-center gap-2 rounded-full border border-brand/20 bg-brand/10 px-4 py-1.5 text-sm font-medium text-brand">
                <span className="size-2 rounded-full bg-brand" />
                Vagas abertas para este mês
              </span>
              <h1 className="mt-6 font-display text-5xl leading-[1.05] font-semibold text-balance md:text-6xl">Agende sua avaliação gratuita e saia com um plano claro</h1>
              <p className="mt-6 max-w-xl text-lg leading-relaxed text-pretty text-muted">Em 30 minutos, um especialista entende o seu momento, responde suas dúvidas e mostra o caminho mais rápido até o resultado que você quer.</p>
              <ul className="mt-8 grid gap-3 text-base">
                {promises.map((promise, i) => (
                  <li key={i} className="flex items-center gap-3">
                    <span className="grid size-6 place-items-center rounded-full bg-brand text-brand-ink">
                      <Check className="size-3.5" strokeWidth={3} />
                    </span>
                    {promise}
                  </li>
                ))}
              </ul>
              <div className="mt-10 flex flex-wrap items-center gap-4">
                <a href="#" data-vz-whatsapp="" data-vz-cta="whatsapp-topo" className="inline-flex items-center gap-2 rounded-full bg-[#25D366] px-6 py-3.5 text-base font-semibold text-white shadow-lg shadow-[#25D366]/25 transition hover:-translate-y-0.5">
                  <MessageCircle className="size-5" />
                  Falar no WhatsApp
                </a>
                <p className="text-sm text-muted">ou preencha o formulário</p>
              </div>
            </div>
            <LeadForm />
          </div>
        </section>

        <section id="beneficios" className="mx-auto max-w-6xl px-6 py-20">
          <h2 className="max-w-2xl font-display text-4xl font-semibold text-balance">Por que tanta gente escolhe a Sua Empresa</h2>
          <div className="mt-12 grid gap-6 md:grid-cols-3">
            {benefits.map((benefit, i) => {
              const Icon = benefitIcons[i % benefitIcons.length];
              return (
                <article key={i} className="rounded-card border border-line bg-white p-8">
                  <span className="grid size-12 place-items-center rounded-2xl bg-brand/10 text-brand">
                    <Icon className="size-6" />
                  </span>
                  <h3 className="mt-6 text-xl font-semibold">{benefit.title}</h3>
                  <p className="mt-3 leading-relaxed text-muted">{benefit.text}</p>
                </article>
              );
            })}
          </div>
        </section>

        <section id="depoimentos" className="bg-night py-20 text-surface">
          <div className="mx-auto max-w-6xl px-6">
            <h2 className="font-display text-4xl font-semibold">Quem já passou por aqui</h2>
            <div className="mt-12 grid gap-6 md:grid-cols-2">
              {testimonials.map((item, i) => (
                <figure key={i} className="rounded-card bg-white/5 p-8 ring-1 ring-white/10">
                  <blockquote className="text-lg leading-relaxed">"{item.quote}"</blockquote>
                  <figcaption className="mt-6 text-sm text-surface/70">{item.author}</figcaption>
                </figure>
              ))}
            </div>
          </div>
        </section>

        <section id="perguntas" className="mx-auto max-w-3xl px-6 py-20">
          <h2 className="text-center font-display text-4xl font-semibold">Perguntas frequentes</h2>
          <div className="mt-10 grid gap-4">
            {questions.map((item, i) => (
              <details key={i} className="rounded-card border border-line bg-white p-6">
                <summary className="cursor-pointer list-none text-lg font-semibold">{item.question}</summary>
                <p className="mt-3 leading-relaxed text-muted">{item.answer}</p>
              </details>
            ))}
          </div>
          <div className="mt-12 text-center">
            <a href="#contato" data-vz-cta="final" className="inline-flex rounded-full bg-brand px-8 py-4 text-base font-semibold text-brand-ink shadow-lg shadow-brand/20 transition hover:-translate-y-0.5">Quero minha avaliação gratuita</a>
          </div>
        </section>
      </main>
      <Footer />
    </>
  );
}
