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

## Mudar um formulário que já existe
Leia antes com site_read (o nó de cada campo, do label e da linha que envolve o campo) e edite com site_edit, campo a campo:
- Ligar um campo ao CRM: set_attr no input com name (lead.name, lead.phone, lead.email, consent, email_opt_in ou field.<chave>) e, se o tipo não servir, set_attr com type. lead.name aceita text; lead.phone aceita tel ou text; lead.email só email; consent e email_opt_in só checkbox. Os campos personalizados do lead deste workspace estão em lead_fields de site_read com references: use o id dele (field.<chave>) e o rótulo como texto do label. Nunca invente uma chave; sem o campo, pergunte ou use outro.
- Obrigatório: set_attr com name "required" e value "" liga; sem value, desliga. consent é sempre obrigatório.
- Rótulo: set_text no elemento de texto do label (o span dentro dele, ou o label separado com for). Nunca set_text num label que envolve o input: o input some junto.
- Novo campo: insert_section com html de uma linha completa, depois da última linha de campo (at.after) e antes do botão, no mesmo estilo das outras linhas: <label class="..."><span>Seu WhatsApp</span><input type="tel" name="lead.phone" autocomplete="tel" required class="..."></label>.
- Reordenar: move com a linha do campo (o label ou o bloco que envolve só aquele campo) before ou after de outra linha.
- Tirar um campo: remove na linha inteira, nunca só no input (o label ficaria sozinho).
- Nome do formulário para a equipe: site_settings, seção forms, form_id do formulário, com "name" (uma linha, até 80 caracteres, como "Fale conosco"). É esse nome que aparece no painel, nos envios e no gatilho dos fluxos; o id do código nunca aparece para as pessoas. Ao trocar a seção forms de um formulário, mande também o que já estava salvo.
Depois de mudar campos, confira open_gaps: identidade (telefone ou e-mail), consentimento e o campo que o canal de acompanhamento usa (lead.phone para WhatsApp, lead.email para e-mail).

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
