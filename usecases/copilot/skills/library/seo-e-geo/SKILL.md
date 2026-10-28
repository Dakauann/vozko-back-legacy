---
name: seo-e-geo
surface: site
description: SEO e GEO de uma página: título e descrição por rota, estrutura de títulos, conteúdo que buscadores e assistentes de IA citam, imagem social e noindex; carregue antes de publicar ou ao otimizar busca.
---

# SEO e GEO

SEO é aparecer no Google; GEO é ser citado por assistentes de IA (ChatGPT, Gemini, Perplexity). Os dois premiam o mesmo: conteúdo claro, específico e fácil de entender por máquina.

## Por rota (site_settings, seção pages)
- title: até 60 caracteres, com o serviço, o lugar e a marca: "Implante dentário em Campinas | Clínica Sorriso".
- description: até 155 caracteres, a promessa e a chamada: "Avaliação gratuita, plano claro e parcelamento. Agende pelo WhatsApp."
- og_media_id: uma imagem da biblioteca de 1200 por 630 px para o compartilhamento no WhatsApp, Instagram e Facebook. Gere com site_generate_image se não houver.
- noindex: true em páginas que não devem aparecer na busca (obrigado, testes).
- canonical: só quando a mesma página existe em outro endereço.

## Estrutura da página
- Um h1 por página com o assunto principal; h2 para cada seção; h3 dentro delas. A ordem importa mais que o tamanho.
- Texto real no HTML: nada importante só dentro de imagem ou carregado por script.
- Alt descritivo nas imagens de conteúdo.
- Links internos com texto que diz o destino.
- lang certo em site.json (pt-BR).

## Conteúdo que é citado
- Responda perguntas diretas com frases autossuficientes: "A avaliação leva 30 minutos e é gratuita." Assistentes de IA citam frases assim.
- Perguntas frequentes com as dúvidas reais, na voz do cliente.
- Fatos verificáveis: endereço completo, bairros atendidos, horário, telefone, registro profissional, preços quando o cliente quiser.
- Nome da empresa e do serviço escritos igual em toda a página.

## Negócio local
- Endereço, cidade e região no texto e no rodapé; um mapa só quando ajuda.
- O mesmo nome, endereço e telefone do perfil da empresa no Google.

## Conferir
site_look aponta h1_missing, h1_repeated e heading_order. Antes de publicar, confira com site_settings que cada rota indexável tem title e description.
