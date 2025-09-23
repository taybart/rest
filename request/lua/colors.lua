-- Color escape
local ce = '\27[0;'
local ceBold = '\27[1;'
local ceItalic = '\27[3;'
local ceUnderlined = '\27[4;'
local ceBlinking = '\27[5;'

return {
  -- Normal Colors
  gray = ce .. '37m',
  purple = ce .. '35m',
  blue = ce .. '34m',
  yellow = ce .. '33m',
  green = ce .. '32m',
  red = ce .. '31m',

  -- Bold Colors
  bold = {
    gray = ceBold .. '37m',
    purple = ceBold .. '35m',
    blue = ceBold .. '34m',
    yellow = ceBold .. '33m',
    green = ceBold .. '32m',
    red = ceBold .. '31m',
  },

  -- Italic Colors
  italic = {
    gray = ceItalic .. '37m',
    purple = ceItalic .. '35m',
    blue = ceItalic .. '34m',
    yellow = ceItalic .. '33m',
    green = ceItalic .. '32m',
    red = ceItalic .. '31m',
  },

  -- Underlined Colors
  underlined = {
    gray = ceUnderlined .. '37m',
    purple = ceUnderlined .. '35m',
    blue = ceUnderlined .. '34m',
    yellow = ceUnderlined .. '33m',
    green = ceUnderlined .. '32m',
    red = ceUnderlined .. '31m',
  },

  -- Blinking Colors
  blinking = {
    gray = ceBlinking .. '37m',
    purple = ceBlinking .. '35m',
    blue = ceBlinking .. '34m',
    yellow = ceBlinking .. '33m',
    green = ceBlinking .. '32m',
    red = ceBlinking .. '31m',
  },

  -- Return to default,
  reset = ce .. '0m',
}
