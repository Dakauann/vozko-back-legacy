---
name: lgpd-em-paginas
surface: site
description: LGPD na página: consentimento no formulário, política de privacidade, banner de cookies quando há pixel ou analytics e o que nunca coletar; carregue antes de publicar um site com formulário ou rastreamento.
---

# LGPD em páginas

Você não dá parecer jurídico; ajuda a montar o site do jeito que a plataforma e a LGPD (Lei 13.709/2018) esperam. Textos legais com dados da empresa vêm do usuário.

## Formulário
- Peça só os dados que o próximo passo precisa (finalidade e necessidade). Nada de CPF, data de nascimento ou dados de saúde num formulário de captura.
- Consentimento explícito: checkbox name="consent" required, desmarcado, com texto claro do que será feito com os dados e por quais canais ("Autorizo o contato por WhatsApp, telefone ou e-mail..."), e link para a política de privacidade. Sem ele, consent_required impede publicar.
- Marketing por e-mail em checkbox separado, opcional (name="email_opt_in").
- Nunca campos de senha ou código de verificação: a plataforma recusa (credential_field_forbidden) e isso parece golpe.

## Política de privacidade
- Uma rota própria (/privacidade) com: quem é o controlador (razão social, CNPJ, endereço e contato do encarregado), quais dados são coletados e para quê, com quem são compartilhados (a plataforma de atendimento, ferramentas de anúncio), por quanto tempo ficam guardados e como pedir acesso, correção ou exclusão.
- Link no rodapé de todas as páginas e no texto do consentimento. Sem link para a política numa página com formulário, privacy_link_missing impede publicar.
- Os dados da empresa vêm do usuário: deixe a página pronta com lacunas visíveis e peça numa pergunta só; nunca invente CNPJ, endereço ou nome de encarregado.

## Cookies e rastreamento
- Pixel da Meta, GA4, GTM, Google Ads, TikTok e Clarity entram por site_settings (seção integrations), nunca por script colado na página.
- Com qualquer integração ligada, o banner de consentimento é obrigatório (seção consent: banner true, text, categories analytics e marketing). Sem ele, consent_banner_missing impede publicar. Os scripts só rodam depois que o visitante aceita.
- Um link "Preferências de cookies" no rodapé com o atributo data-vz-consent-open reabre o painel.
- A contagem de visitas da própria plataforma é agregada e sem identificação, por isso não depende do banner.

## Setores sensíveis
Saúde, advocacia e serviços financeiros pedem ainda mais cuidado com o que se coleta e com as promessas; carregue a habilidade do nicho quando for o caso.
