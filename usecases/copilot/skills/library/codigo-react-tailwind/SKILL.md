---
name: codigo-react-tailwind
surface: site
description: Como escrever uma página React com Tailwind no formato do editor: pastas, rotas, tema, bibliotecas aceitas e o código que o usuário consegue editar à mão; carregue antes de criar arquivos numa página react.
---

# Código React com Tailwind

A página é um projeto React 19 com Tailwind CSS 4 que o construtor da plataforma compila e pré-renderiza. O usuário edita a mesma fonte clicando na prévia e no modo Código, então o código precisa ser simples de editar por clique.

## Estrutura
- site.json: nome, lang e routes, cada uma com path (/, /obrigado) e page (o arquivo da rota, como src/routes/index.jsx). Rota nova = arquivo novo mais uma entrada em routes.
- src/main.jsx: entrada (hidrata ou renderiza App). Não mude sem motivo.
- src/App.jsx: mapeia o caminho para o componente da rota.
- src/routes/*.jsx: uma página por rota, com export default function.
- src/components/*.jsx: partes reutilizadas (Header, Footer, LeadForm). Componentize o que se repete entre rotas; uma seção usada uma vez pode ficar na própria rota.
- src/index.css: @import "tailwindcss" e o bloco @theme com os tokens. knobs.json: ajustes que o usuário mexe em sliders.
- Caminhos ficam sob src/, com .jsx, .tsx, .js, .ts ou .css. Imports entre arquivos são relativos ("../components/Header.jsx").

## Bibliotecas aceitas
react, react-dom, framer-motion, motion, lucide-react, clsx, tailwind-merge, class-variance-authority, gsap, swiper, embla-carousel-react, lottie-web, three e os pacotes @radix-ui/react-* (accordion, dialog, tabs, dropdown-menu, popover, tooltip, select, checkbox, radio-group, switch, label, separator, avatar, navigation-menu, collapsible, slot). Outro pacote é recusado pelo construtor. Ícones: lucide-react.

## Tema e CSS
- Cores, fontes e raios vivem em @theme como variáveis (--color-brand, --font-display, --radius-card) e viram classes do Tailwind (bg-brand, font-display, rounded-card). Mudar o visual = set_token no tema, não trocar classes em cem lugares.
- @import só de "tailwindcss" ou de arquivos do próprio projeto; uma fonte externa por @import url() é recusada. Use as famílias declaradas no tema.
- CSS próprio só para o que o Tailwind não faz (keyframes, um efeito específico), em src/index.css.

## Código editável à mão (obrigatório)
- className literal: escreva as classes inteiras como string no próprio elemento (className="mt-6 text-lg text-muted"). Não monte classes com template strings, não guarde classes em variáveis, não esconda em helpers; o inspetor só edita o que está literal. clsx ou cva apenas quando há variação real de estado, e mesmo assim com strings literais.
- Texto como texto: títulos, parágrafos e botões escritos direto no JSX (<h1>Agende sua avaliação</h1>). Listas repetidas (benefícios, depoimentos, perguntas) podem ficar num array de objetos com strings literais no mesmo arquivo ou num único módulo de conteúdo (src/content.js) só com literais, renderizado com map e key; nada calculado, nada vindo de rede.
- Imagens com src literal: <img src="https://..." alt="..." /> com a URL da biblioteca (media_url). Nada de import de imagem nem src calculado. replace_image troca por media_id.
- Links com href literal. Âncoras internas (#contato) para rolar até uma seção, rotas (/obrigado) para outra página.
- Um componente por arquivo, com nome claro; props simples.
- Sem fetch, sem estado global, sem efeitos que mudam o texto depois de carregar: a página é pré-renderizada e o texto precisa existir no HTML.

## Elementos da plataforma
- Formulário: <form data-vz-form="contato"> com campos name="lead.name", "lead.phone", "lead.email" e extras "field.<chave>"; a plataforma envia, protege e entrega o lead. Não escreva onSubmit nem action. Consentimento: <label><input type="checkbox" name="consent" required /> texto do consentimento com link para /privacidade</label>. Detalhes em formularios-e-captura.
- WhatsApp: <a href="#" data-vz-whatsapp="ID_DO_NÚMERO" data-vz-text="mensagem pronta">. A plataforma troca o href pelo link certo ao publicar. O id do número sai de site_read com references; se houver mais de um número, pergunte qual usar; sem id, o link fica como pendência.
- Chamadas: data-vz-cta="nome-da-posicao" em cada botão ou link de chamada, para a análise contar cliques.

## Editando com site_edit
- Leia o trecho com site_read (path e linhas) e use edit_file com um old exato e único; inclua uma ou duas linhas de contexto quando o trecho se repete.
- write_file para arquivo novo; não reescreva um arquivo inteiro para mudar uma linha.
- Para só texto, classes, atributo ou imagem de um elemento já conhecido (pela seleção do usuário ou pelos marks do site_look), as ops visuais (set_text, set_classes, set_attr, replace_image) são mais baratas e seguras.
- Um lote por etapa lógica (um componente novo mais a rota que o usa). Depois, site_look.

## Erros comuns
- build_errors no retorno: leia o caminho e a linha, corrija no mesmo lote seguinte.
- invisible_without_js: texto que só aparece depois de uma animação; veja motion-web.
- Hidratação: nada que mude entre servidor e navegador (datas, Math.random, window no render).
