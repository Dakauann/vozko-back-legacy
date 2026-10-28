---
name: formularios-e-captura
surface: site
description: Formulários e WhatsApp na página: campos que convertem, o contrato data-vz-form, consentimento, para onde vai o lead, sucesso e botão de WhatsApp; carregue antes de criar ou mudar um formulário.
---

# Formulários e captura

## Quantos campos
- Cada campo a mais custa conversão. Para agendar ou pedir contato: nome e WhatsApp. E-mail só quando o próximo passo é por e-mail.
- Pergunte o essencial para qualificar (serviço de interesse, cidade) com um select ou botões de opção, nunca um texto livre longo.
- Formulários longos (orçamento detalhado) funcionam em etapas curtas ou depois de um primeiro contato no WhatsApp.

## Contrato da plataforma
- O formulário é <form data-vz-form="contato"> (id em minúsculas, números, - e _; único na página). Sem action, sem onSubmit, sem script: a plataforma envia com proteção contra robôs, registra a origem da visita (UTM, anúncio) e entrega o lead.
- Campos com name: lead.name, lead.phone, lead.email para a identidade; field.<chave> para dados extras (field.servico, field.cidade). O formulário precisa de telefone ou e-mail; sem nenhum dos dois, o retorno aponta form_identity_missing.
- Tipos certos ajudam no celular: type="tel" com autocomplete="tel", type="email" com autocomplete="email", autocomplete="name". required só no que é obrigatório mesmo.
- Cada campo dentro de um label com o texto visível (placeholder não substitui label).
- Botão type="submit" com uma chamada específica ("Agendar minha avaliação").

## Consentimento (obrigatório)
- Um checkbox name="consent" required dentro de um label com o texto do consentimento: o que a empresa fará com os dados e por quais canais, com link para a política de privacidade. Sem ele, consent_required impede publicar.
- E-mails de marketing pedem um checkbox separado name="email_opt_in", desmarcado.
- A página precisa de um link para a política de privacidade (rota /privacidade com os dados reais da empresa); sem ele, privacy_link_missing. Veja lgpd-em-paginas.

## Para onde vai o lead
Configure com site_settings, seção forms e form_id igual ao data-vz-form:
- funnel_id e stage_id: o funil e a etapa onde o lead entra.
- owner: quem atende (kind unassigned, user com o id do membro, ou roulette com o id do departamento).
- workflow_id com follow_up (kind whatsapp, unofficial_whatsapp ou email e o id do canal): a automação que responde o lead; um workflow sem follow_up é recusado.
- success: kind message (com message), redirect (com route, como /obrigado) ou whatsapp (com channel).
- notify: avisa a equipe a cada envio.
Ids de funil, etapa, membro, departamento, workflow e canal vêm do usuário ou das telas do painel; nunca invente. Sem esses dados, deixe o formulário pronto e peça ao usuário numa pergunta só.

## Depois do envio
- Uma página de obrigado (noindex) que confirma, diz o próximo passo e o prazo, e oferece o WhatsApp.
- Mensagem de sucesso curta quando não há página de obrigado.

## WhatsApp na página
- Botão ou link <a href="#" data-vz-whatsapp="ID_DO_NÚMERO" data-vz-text="Olá! Vim pela página e quero agendar uma avaliação.">: a plataforma troca o href pelo link do número ao publicar. O id do número sai de site_read com references; se houver vários, pergunte qual usar. Sem id, o link fica vazio e vira pendência.
- Mensagem pronta curta, que diz de onde a pessoa veio e o que quer.
- Marque com data-vz-cta para contar os cliques.
- Botão flutuante de WhatsApp só quando não compete com a chamada principal.

## Conferir
open_gaps lista o que falta (identidade, consentimento, link de privacidade, links vazios). site_look mostra o formulário no celular: teclado certo, campos grandes (altura de pelo menos 44 px), erro visível.
