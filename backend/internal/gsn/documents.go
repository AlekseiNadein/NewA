package gsn

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type HierarchyDocument struct {
	FileName string
	Content  []byte
}

func (s *Service) GetHierarchyDocument(ctx context.Context, supplementCode, hierarchyCode string) (HierarchyDocument, error) {
	if !s.Configured() {
		return HierarchyDocument{}, ErrNotConfigured
	}

	supplementCode = strings.TrimSpace(supplementCode)
	hierarchyCode = strings.TrimSpace(hierarchyCode)
	if supplementCode == "" || hierarchyCode == "" {
		return HierarchyDocument{}, fmt.Errorf("supplement and hierarchy code are required")
	}

	var documentRef, documentFile string
	err := s.db.QueryRowContext(ctx, `
		SELECT document_ref, document_file
		FROM gsn.hierarchy
		WHERE supplement_code = $1
			AND code = $2
			AND node_type = 'Документ'
			AND document_ref <> ''
	`, supplementCode, hierarchyCode).Scan(&documentRef, &documentFile)
	if err == sql.ErrNoRows {
		return HierarchyDocument{}, fmt.Errorf("document not found")
	}
	if err != nil {
		return HierarchyDocument{}, fmt.Errorf("query gsn hierarchy document: %w", err)
	}

	fileName := strings.TrimSpace(documentFile)
	if fileName == "" {
		fileName = strings.TrimSpace(documentRef)
	}
	if fileName == "" {
		return HierarchyDocument{}, fmt.Errorf("document file is not set")
	}

	var content []byte
	err = s.db.QueryRowContext(ctx, `
		SELECT content
		FROM gsn.documents
		WHERE file_name = $1
	`, fileName).Scan(&content)
	if err == sql.ErrNoRows {
		return HierarchyDocument{}, fmt.Errorf("pdf file is not imported")
	}
	if err != nil {
		return HierarchyDocument{}, fmt.Errorf("query gsn document: %w", err)
	}

	return HierarchyDocument{
		FileName: fileName,
		Content:  content,
	}, nil
}
