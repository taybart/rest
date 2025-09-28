-- Color escape
local ce = '\27[0;'
local ceBold = '\27[1;'
local ceItalic = '\27[3;'
local ceUnderlined = '\27[4;'
local ceBlinking = '\27[5;'

local colors = {
  -- Normal Colors
  gray = ce .. '37m',
  purple = ce .. '35m',
  blue = ce .. '34m',
  yellow = ce .. '33m',
  green = ce .. '32m',
  red = ce .. '31m',

  -- Bold Colors
  bold = {
    -- e's are non colored escapes (used in colors.f)
    e = '\27[1m',
    gray = ceBold .. '37m',
    purple = ceBold .. '35m',
    blue = ceBold .. '34m',
    yellow = ceBold .. '33m',
    green = ceBold .. '32m',
    red = ceBold .. '31m',
  },

  -- Italic Colors
  italic = {
    e = '\27[3m',
    gray = ceItalic .. '37m',
    purple = ceItalic .. '35m',
    blue = ceItalic .. '34m',
    yellow = ceItalic .. '33m',
    green = ceItalic .. '32m',
    red = ceItalic .. '31m',
  },

  -- Underlined Colors
  underlined = {
    e = '\27[4m',
    gray = ceUnderlined .. '37m',
    purple = ceUnderlined .. '35m',
    blue = ceUnderlined .. '34m',
    yellow = ceUnderlined .. '33m',
    green = ceUnderlined .. '32m',
    red = ceUnderlined .. '31m',
  },

  -- Blinking Colors
  blinking = {
    e = '\27[5m',
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

function colors.f(co, text)
  if type(co) == 'string' then
    return co .. text .. colors.reset
  end
  if type(co) == 'table' then
    local out = ''
    for _, v in pairs(co) do
      out = out .. v
    end
    return out .. text .. colors.reset
  end
end

function colors.hyperlink(url, text, color)
  local ret = '\27]8;;' .. url .. '\27\\' .. text .. '\27]8;;\27\\'
  if color then
    return color .. ret .. colors.reset
  end
  return ret
end

return colors
