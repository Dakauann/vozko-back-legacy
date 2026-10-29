---
name: codigo-html-tailwind
surface: site
description: Como escrever um site HTML com Tailwind no formato do editor: pages, partials, scripts, tema e o markup que o usuário edita à mão; carregue antes de criar arquivos num site html.
---

# Código HTML com Tailwind

O site é HTML estático com Tailwind CSS 4, montado pelo construtor da plataforma. O usuário edita a mesma fonte clicando na prévia e no modo Código.

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
- Chat do site: <vz-webchat widget="CHAVE"></vz-webchat> uma vez na página, sem script. A chave sai de site_read com references (webchat_widgets); sem chat cadastrado, não coloque o elemento. Para todas as páginas, prefira site_settings na seção site com webchat_widget. Na prévia o chat aparece igual ao publicado, em modo de demonstração: nenhuma mensagem é enviada nem chega à caixa de entrada. Não escreva iframe nem script do chat: a publicação recusa.
- Agendamento: <vz-booking booking="ID"></vz-booking> mostra os horários livres da agenda Google do workspace e reserva o escolhido, sem script e sem form próprio. O ID é um bloco da seção bookings de site_settings (site_read com references traz booking_blocks e google_calendar_connected); sem agenda conectada, não coloque o elemento. O destino do lead fica na seção forms com o mesmo ID, como um formulário. O conteúdo de dentro do elemento aparece até os horários carregarem. Ele herda --color-brand, --color-brand-ink, --color-line e --radius-card do tema; com o atributo unstyled, estilize pelos atributos data-vz-* que ele gera. Na prévia os horários são de exemplo e nada é reservado.
- Acordeão sem script: details e summary.

## Editando com site_edit
- Leia com site_read (path e linhas) e use edit_file com um old exato e único.
- write_file para página ou partial novos; partial novo entra com vz-include onde for usado.
- Para texto, classes, atributo ou imagem de um elemento já conhecido, use set_text, set_classes, set_attr e replace_image.
- insert_section com html coloca uma seção inteira antes, depois ou dentro de um node, ou no fim de uma página (at.page).
- Depois de cada etapa, site_look.

## Telas: computador primeiro
O editor trabalha computador primeiro, como as pessoas veem: a classe sem prefixo vale para todas as telas e é o desenho do computador; max-lg: muda o tablet e o celular; max-md: muda só o celular. Use o mesmo formato:
- Em set_classes, não mande breakpoint (ele escreve md: e lg:, que valem da largura para cima). Mande a classe completa em add e a que ela substitui em remove: add ["max-md:text-4xl"], remove ["max-md:text-5xl"].
- Para mudar todas as telas, troque a classe sem prefixo; um max-lg: ou max-md: que já existe continua valendo naquela tela.
- Páginas antigas com md: e lg: continuam certas. Ao mudar um valor nelas, reescreva o grupo inteiro (base, max-lg:, max-md:) mantendo o que cada tela mostra hoje, para não sobrar classe que contradiz a nova.
- Código novo segue o mesmo formato: py-24 max-md:py-14, text-6xl max-lg:text-5xl max-md:text-4xl.
