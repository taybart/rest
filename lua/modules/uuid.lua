return {
  newv4 = function()
    -- seed every time because we don't have a global start at the moment
    math.randomseed(os.time())
    local template = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'
    return (
      template:gsub('[xy]', function(c)
        local bits = (c == 'x') and math.random(0, 0xf) or math.random(8, 0xb)
        return string.format('%x', bits)
      end)
    )
  end,
  is_uuidv4 = function(str)
    -- lua patterns are stupid
    local pattern = '%w%w%w%w%w%w%w%w%-%w%w%w%w%-4%w%w%w%-[89abAB]%w%w%w%-%w%w%w%w%w%w%w%w%w%w%w%w'
    return str:match(pattern)
  end,
}
