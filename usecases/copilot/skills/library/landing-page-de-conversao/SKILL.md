---
name: landing-page-de-conversao
surface: site
description: Estrutura de uma landing page que converte: oferta, ordem das seções, prova, objeções, ritmo de chamadas e coerência com o anúncio; carregue ao planejar ou revisar uma página.
---

# Landing page de conversão

Uma landing page tem um único trabalho: levar quem chega a dar um próximo passo (agendar, pedir orçamento, chamar no WhatsApp, comprar). Tudo o que não ajuda nisso sai.

## Antes da primeira seção
Responda, com o usuário se faltar algo (uma pergunta só, com as opções que você imagina):
- Quem chega e de onde (anúncio da Meta, Google, Instagram, indicação)? O que essa pessoa já sabe?
- Qual é a oferta concreta (avaliação gratuita, orçamento em 24 h, 10% na primeira compra)? Oferta vaga não converte.
- Qual é o próximo passo único e para onde vai o lead (formulário que cai num funil, WhatsApp)?
- Que provas existem de verdade (números, depoimentos reais, logos de clientes, certificações, tempo de mercado)? Nunca invente prova.

## Ordem que funciona (adapte, não copie)
1. Herói: promessa clara do resultado para aquele público, uma linha de apoio que diz como, a chamada principal visível sem rolar e um sinal de confiança perto dela.
2. Prova rápida: números, avaliações ou logos logo abaixo do herói.
3. Problema e desejo: mostre que você entende a situação de quem chega, nas palavras dele.
4. Como funciona: três ou quatro passos simples até o resultado.
5. Benefícios: o que a pessoa ganha, não a lista de recursos.
6. Prova profunda: depoimentos específicos, com nome e contexto, casos, antes e depois quando a regra do setor permite.
7. Objeções: perguntas frequentes que respondem medos reais (preço, prazo, risco, "funciona para mim?").
8. Oferta e garantia: repita o que a pessoa recebe e o que reduz o risco.
9. Chamada final com o formulário ou o botão.
Rodapé com dados da empresa, política de privacidade e contato.

## Ritmo de chamadas
- A mesma chamada, com o mesmo texto, aparece no herói, no meio e no fim. Uma chamada principal por página; secundárias (WhatsApp, por exemplo) com menos peso visual.
- Marque cada chamada com data-vz-cta e um nome que diga onde ela está (topo, meio, final): a análise conta os cliques por posição.
- No celular, a chamada principal fica ao alcance do polegar; um botão fixo no rodapé da tela ajuda em páginas longas.

## Formulário ou WhatsApp
- Formulário curto quando o lead precisa entrar num funil com dados (nome e WhatsApp bastam na maioria dos casos). Cada campo a mais derruba conversão; peça o resto na conversa. Veja formularios-e-captura.
- Botão de WhatsApp quando a conversa é o produto (orçamento, dúvida rápida). Os dois juntos funcionam bem: formulário como principal e WhatsApp como alternativa.
- Defina para onde vão os leads com site_settings (seção forms) antes de publicar. Um formulário com funil precisa de responsável ou de fluxo; o fluxo escolhido precisa ser uma automação cujo gatilho é este formulário.
- Chat do site como terceira via: site_settings na seção site com webchat_widget (chave de references, webchat_widgets) coloca o chat em todas as páginas.
- Agendamento quando o próximo passo é marcar horário (consulta, avaliação, reunião, visita): o bloco vz-booking (veja codigo-html-tailwind ou codigo-react-tailwind) troca o vai e vem da conversa por um horário reservado na hora. Crie o bloco em site_settings seção bookings (duração, intervalo entre reuniões, antecedência mínima, dias à frente, horário de atendimento, título do evento com {nome}, Meet, campos extras, consentimento) e o destino do lead na seção forms com o mesmo id. O fluxo do formulário recebe field.horario, field.horario_fim e field.link_reuniao para confirmar e lembrar pelo WhatsApp.
- Antes de publicar, confira título e descrição de cada rota (seção pages) e o favicon da marca (seção site, favicon_media_id de uma imagem da biblioteca); sem favicon a aba do navegador fica genérica.

## Coerência com o anúncio
- Quem clica num anúncio espera ver a mesma promessa, a mesma oferta e o mesmo visual logo de cara. Um título diferente do anúncio faz a pessoa voltar.
- Uma página por oferta ou público quando as campanhas são diferentes; é melhor que uma página que tenta servir a todos.
- Pixel e Google Analytics entram em site_settings (seção integrations); com integrações ligadas, o banner de consentimento é obrigatório (veja lgpd-em-paginas).

## Páginas além da principal
- Página de obrigado (rota própria, noindex): confirma, diz o que acontece agora e em quanto tempo, e oferece um passo extra (WhatsApp, Instagram). O sucesso do formulário pode redirecionar para ela.
- Política de privacidade com os dados reais da empresa, ligada no rodapé e no consentimento do formulário.

## Vídeos e mapas incorporados
Um iframe só publica com src de www.youtube-nocookie.com/embed/..., player.vimeo.com/video/... ou www.google.com/maps/embed?...; troque links do youtube.com pelo endereço youtube-nocookie. Qualquer outro iframe, ou um iframe com srcdoc, impede a publicação (frame_source_forbidden).

## Setores regulados
Saúde, odontologia, advocacia e imóveis têm regras de publicidade que valem também na página: carregue a habilidade do nicho (nicho-medicina, nicho-odontologia, nicho-advocacia, nicho-imobiliario) antes de escrever.

## Revisão final
Olhe com site_look nos dois tamanhos e responda: a promessa está clara em três segundos? Existe um só próximo passo óbvio? Cada seção tira uma dúvida ou aumenta o desejo? Há algo inventado? open_gaps está vazio?
