---
name: acessibilidade-web
surface: site
description: Página acessível: contraste, alt, títulos em ordem, foco visível, toque no celular, formulários com label e os checks do site_look; carregue antes de publicar ou quando site_look trouxer problemas de acessibilidade.
---

# Acessibilidade web

Página acessível converte mais: funciona para quem enxerga pouco, usa leitor de tela, navega pelo teclado ou está no sol com o celular. A Lei Brasileira de Inclusão também pede isso.

## Texto e cor
- Contraste de pelo menos 4,5:1 no texto normal e 3:1 em texto grande (24 px, ou 18,5 px em negrito). site_look aponta contrast_low com o node; corrija no token da cor quando o problema se repete.
- Texto sobre foto precisa de um véu escuro ou de um fundo atrás do texto.
- Nunca use só a cor para dizer algo (erro em vermelho também leva texto).
- Corpo de texto com pelo menos 16 px no celular.

## Estrutura
- Um h1, depois h2 e h3 em ordem. header, nav, main e footer em volta das partes da página.
- Links e botões com texto que diz o que fazem; ícone sozinho precisa de aria-label. link_unnamed aponta os que faltam.
- Âncoras para rolar são links (a href="#contato"); ações são botões (button).

## Imagens
- alt descreve o que a imagem mostra e importa para a página ("Dentista atendendo paciente no consultório"); imagem decorativa leva alt vazio (alt=""). Sem alt, alt_missing impede publicar.

## Teclado e foco
- Tudo que se clica também funciona com Tab e Enter. Foco sempre visível: focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2.
- Diálogos e menus com os componentes @radix-ui (React) ou details e summary (HTML), que já cuidam do teclado.

## Celular
- Alvos de toque de pelo menos 44 por 44 px, com espaço entre eles.
- Nada de estouro horizontal (overflow_horizontal no site_look).
- Zoom permitido.

## Formulários
- Cada campo com label visível ligado a ele, tipo e autocomplete certos, mensagem de erro em texto.

## Movimento
- Respeite prefers-reduced-motion; veja motion-web.

## Conferir
site_look aponta contraste baixo (contrast_low), estouro no celular e imagens pequenas; site_read e site_edit trazem h1, ordem dos títulos e links sem nome. Corrija o que afeta mais gente primeiro e olhe de novo. Foco visível, ordem do teclado e labels você confere lendo o código.
