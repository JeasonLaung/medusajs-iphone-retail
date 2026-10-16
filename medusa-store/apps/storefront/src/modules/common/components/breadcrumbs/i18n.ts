export const getBreadcrumbCopy = (locale: string) => ({
  label: locale.toLowerCase().startsWith("zh") || locale.endsWith("-zh")
    ? "面包屑导航"
    : "Breadcrumb",
})
