import Footer from "../components/Footer.jsx";
import Header from "../components/Header.jsx";

const sections = [
  { title: "Quais dados coletamos", text: "Nome, telefone, e-mail e as informações que você escrever no formulário, além de dados de navegação agregados e anônimos sobre as visitas." },
  { title: "Para que usamos", text: "Para responder ao seu pedido, agendar o atendimento e enviar informações sobre o serviço solicitado. Não vendemos seus dados." },
  { title: "Seus direitos", text: "Você pode pedir acesso, correção ou exclusão dos seus dados e revogar o consentimento a qualquer momento pelos nossos canais de atendimento." },
  { title: "Contato", text: "Fale com a gente pelo e-mail contato@suaempresa.com.br." },
];

export default function Privacidade() {
  return (
    <>
      <Header />
      <main className="bg-surface font-sans text-ink">
        <article className="mx-auto max-w-3xl px-6 py-20 leading-relaxed">
          <h1 className="font-display text-4xl font-semibold">Política de Privacidade</h1>
          <p className="mt-6 text-muted">Esta política explica como a Sua Empresa trata os dados pessoais enviados por esta página, de acordo com a Lei Geral de Proteção de Dados (Lei 13.709/2018).</p>
          {sections.map((section, i) => (
            <section key={i}>
              <h2 className="mt-10 text-2xl font-semibold">{section.title}</h2>
              <p className="mt-3 text-muted">{section.text}</p>
            </section>
          ))}
        </article>
      </main>
      <Footer />
    </>
  );
}
