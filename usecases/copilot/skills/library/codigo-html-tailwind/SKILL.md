---
name: codigo-html-tailwind
surface: site
description: Como escrever uma página HTML com Tailwind no formato do editor: pages, partials, scripts, tema e o markup que o usuário edita à mão; carregue antes de criar arquivos numa página html.
---

# Código HTML com Tailwind

A página é HTML estático com Tailwind CSS 4, montado pelo construtor da plataforma. O usuário edita a mesma fonte clicando na prévia e no modo Código.

## Estrutura
- site.json: nome, lang e routes, cada uma com path (/, /obrigado) e page (pages/index.html). Rota nova = página nova mais uma entrada em routes.
- pages/*.html: o conteúdo de cada rota, sem html, head nem body (a plataforma monta o documento, o título e as metas a partir de site_settings).
- partials/*.html: trechos repetidos, incluídos com <vz-include src="partials/header.html"></vz-include>.
- scripts/*.js: comportamento próprio, quando inevitável, como módulo; nunca script inline nem atributos onclick.
- theme.css: @import "tailwindcss" e o bloco @theme com os tokens. knobs.json: ajustes que o usuário mexe em sliders.
- Nomes de arquivo em minúsculas, números, - e _.

## Tema e CSS
- Cores, fontes e raios vivem em @theme (--color-brand, --font-display, --radius-card) e viram classes (bg-brand, font-display, rounded-card). Mudar o visual = set_token no tema.
- @import só de "tailwindcss" ou de arquivos do projeto; fonte externa por @import url() é recusada. Use as famílias do tema.
- Bibliotecas de comportamento aceitas nos scripts: gsap, motion, swiper, lottie-web, three e lucide. Outras são recusadas.

## Markup editável à mão (obrigatório)
- class literal em cada elemento, com as classes inteiras. Nada de classes montadas por script.
- Texto direto no HTML, em elementos semânticos: header, main, section com id, h1 a h3 em ordem, p, ul, a, button.
- Imagens com src literal e alt: <img src="https://..." alt="..."> com a URL da biblioteca (media_url). replace_image troca por media_id.
- Links com href literal: âncoras (#contato) ou rotas (/obrigado).
- Seções com id curto e legível (beneficios, depoimentos, perguntas); viram o mapa no site_read e âncoras de navegação.
- Script só para comportamento; o conteúdo existe no HTML sem JavaScript.

## Elementos da plataforma
- Formulário: <form data-vz-form="contato"> com campos name="lead.name", "lead.phone", "lead.email" e extras "field.<chave>". Sem action e sem script: a plataforma envia, protege e entrega o lead. Consentimento: <label><input type="checkbox" name="consent" required> texto do consentimento com link para /privacidade</label>. Detalhes em formularios-e-captura.
- WhatsApp: <a href="#" data-vz-whatsapp="ID_DO_NÚMERO" data-vz-text="mensagem pronta">. O link certo entra ao publicar; o id do número sai de site_read com references; se houver mais de um, pergunte qual usar.
- Chamadas: data-vz-cta="nome-da-posicao" em cada botão ou link de chamada.
- Acordeão sem script: details e summary.

## Editando com site_edit
- Leia com site_read (path e linhas) e use edit_file com um old exato e único.
- write_file para página ou partial novos; partial novo entra com vz-include onde for usado.
- Para texto, classes, atributo ou imagem de um elemento já conhecido, use set_text, set_classes, set_attr e replace_image.
- insert_section com html coloca uma seção inteira antes, depois ou dentro de um node, ou no fim de uma página (at.page).
- Depois de cada etapa, site_look.
