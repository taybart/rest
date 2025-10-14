local M = {}

function M.is_expected_response()
  local expect = rest.req.expect
  if expect == nil then
    return true
  end
  local res = rest.res
  if expect.status ~= res.status then
    fail(('unexpected status code %d != %d'):format(expect.status, res.status))
    return false
  end
  if expect.body ~= '' and expect.body ~= res.body then
    fail(('body does not match expectation %s != %s'):format(expect.body, res.body))
    return false
  end
  if expect.headers ~= nil then
    for k, v in pairs(expect.headers) do
      local finalkey, values = M.get_header_values(k)
      if values == nil then
        fail(('header %s does not present "%s" != "%s"'):format(finalkey))
        return false
      end
      local matches = false
      for _, value in ipairs(values) do
        if v == value then
          matches = true
        end
      end
      if not matches then
        -- small assumption that header is standalone for usablilty
        fail(('header %s does not match "%s" != "%s"'):format(finalkey, v, values[1]))
        return false
      end
    end
  end
  return true
end

function M.get_header(_key)
  local key, header = M.get_header_values(_key)
  if header == nil then
    return _key, nil
  end
  return key, header[1]
end

function M.get_header_values(key)
  local headers = rest.res.headers
  if headers[key] ~= nil then
    return key, headers[key]
  end
  local upperkey = key:upper()
  if headers[upperkey] ~= nil then
    return upperkey, headers[upperkey]
  end
  local lowerkey = key:lower()
  if headers[lowerkey] ~= nil then
    return lowerkey, headers[lowerkey]
  end
  local mixedkey = key:gsub("(%a)([%w_']*)", function(a, b)
    return string.upper(a) .. b
  end)
  if headers[mixedkey] ~= nil then
    return mixedkey, headers[mixedkey]
  end
  return key, nil
end

return M
