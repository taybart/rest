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
  if expect.body ~= nil and expect.body ~= res.body then
    fail(('body does not match expectation %s != %s'):format(expect.body, res.body))
    return false
  end
  if expect.headers ~= nil then
    for k, v in pairs(expect.headers) do
      local finalkey, value = M.get_header(k)
      if v ~= value then -- FIXME: assumption
        fail(('header %s does not match "%s" != "%s"'):format(finalkey, v, res.headers[finalkey]))
        return false
      end
      -- TODO: should we be good boys and actually include all headers?
      -- if res.headers[k] ~= nil then
      --   if v ~= res.headers[k]['1'] then -- FIXME: assumption
      --     fail(('header %s does not match %s != %s'):format(k, v, res.headers[k]))
      --   end
      -- end
    end
  end
  return true
end

function M.get_header(key)
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
